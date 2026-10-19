package reader

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	"taskpicker/internal/model"
)

// XML nodes use local names so the standard DrawingML and WPS namespace aliases
// can differ without changing the picture-to-cell mapping.
type xmlNode struct {
	Name     string
	Attrs    []xml.Attr
	Children []xmlNode
	Text     string
}

func (node *xmlNode) UnmarshalXML(decoder *xml.Decoder, start xml.StartElement) error {
	node.Name = start.Name.Local
	node.Attrs = start.Attr
	for {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch value := token.(type) {
		case xml.StartElement:
			var child xmlNode
			if err := decoder.DecodeElement(&child, &value); err != nil {
				return err
			}
			node.Children = append(node.Children, child)
		case xml.CharData:
			node.Text += string(value)
		case xml.EndElement:
			return nil
		}
	}
}

func (node xmlNode) attr(name string) string {
	for _, a := range node.Attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func (node xmlNode) child(name string) xmlNode {
	for _, child := range node.Children {
		if child.Name == name {
			return child
		}
	}
	return xmlNode{}
}

func (node xmlNode) descendants(name string) []xmlNode {
	var out []xmlNode
	for _, child := range node.Children {
		if child.Name == name {
			out = append(out, child)
		}
		out = append(out, child.descendants(name)...)
	}
	return out
}

type excelArchive struct{ files map[string]*zip.File }

func (z excelArchive) read(name string, limit int64) ([]byte, error) {
	file := z.files[name]
	if file == nil {
		return nil, fmt.Errorf("Excel 内缺少 %s。", name)
	}
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return LimitedRead(reader, limit)
}

func (z excelArchive) node(name string) (xmlNode, error) {
	var node xmlNode
	raw, err := z.read(name, 32*1024*1024)
	if err != nil {
		return node, err
	}
	err = xml.Unmarshal(raw, &node)
	return node, err
}

func (z excelArchive) rels(part string) (map[string]string, error) {
	result := map[string]string{}
	relpath := path.Join(path.Dir(part), "_rels", path.Base(part)+".rels")
	if z.files[relpath] == nil {
		return result, nil
	}
	root, err := z.node(relpath)
	if err != nil {
		return nil, err
	}
	for _, rel := range root.Children {
		if rel.attr("TargetMode") == "External" {
			continue
		}
		target := rel.attr("Target")
		if strings.HasPrefix(target, "/") {
			target = path.Clean(strings.TrimPrefix(target, "/"))
		} else {
			target = path.Clean(path.Join(path.Dir(part), target))
		}
		if strings.HasPrefix(target, "../") || target == ".." {
			return nil, errors.New("Excel 图片关系地址异常。")
		}
		result[rel.attr("Id")] = target
	}
	return result, nil
}

func IntText(value string) int { n, _ := strconv.Atoi(value); return n }

func dimension(value string) float64 { n, _ := strconv.ParseFloat(value, 64); return n / 9525 }

var dispimgFormula = regexp.MustCompile(`(?i)DISPIMG\s*\(\s*"([^"]+)"`)

var imageFormula = regexp.MustCompile(`(?i)\bIMAGE\s*\(`)

func readExcelPictures(ctx context.Context, filename string) (map[string][]model.Picture, []string, error) {
	reader, err := zip.OpenReader(filename)
	if err != nil {
		return nil, nil, err
	}
	defer reader.Close()
	z := excelArchive{files: map[string]*zip.File{}}
	var expanded uint64
	for _, file := range reader.File {
		expanded += file.UncompressedSize64
		if expanded > 256*1024*1024 {
			return nil, nil, errors.New("Excel 解压后超过 256 MB，请缩小文件范围。")
		}
		z.files[file.Name] = file
	}
	workbook, err := z.node("xl/workbook.xml")
	if err != nil {
		return nil, nil, err
	}
	bookrels, err := z.rels("xl/workbook.xml")
	if err != nil {
		return nil, nil, err
	}
	cellpics := map[string]string{}
	mapped := map[string]bool{}
	pictures := map[string][]model.Picture{}
	warnings := []string{}
	for part := range z.files {
		if !strings.HasSuffix(strings.ToLower(part), "/cellimages.xml") {
			continue
		}
		root, err := z.node(part)
		if err != nil {
			return nil, nil, err
		}
		rels, err := z.rels(part)
		if err != nil {
			return nil, nil, err
		}
		for _, pic := range root.descendants("pic") {
			desc, blips := pic.descendants("cNvPr"), pic.descendants("blip")
			if len(desc) > 0 && len(blips) > 0 {
				target := rels[blips[0].attr("embed")]
				if target != "" {
					cellpics[desc[0].attr("name")] = target
				}
			}
		}
	}
	addPicture := func(sheet, target string, row, col int, width, height float64) {
		if ctx.Err() != nil {
			return
		}
		raw, err := z.read(target, 30*1024*1024)
		var pic model.Picture
		if err == nil {
			pic, err = PngPicture(raw, row, col, width, height)
		}
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("「%s」第 %d 行图片无法解码：%v", sheet, row, err))
			return
		}
		pictures[sheet] = append(pictures[sheet], pic)
		mapped[target] = true
	}
	for _, sheet := range workbook.child("sheets").Children {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		name := sheet.attr("name")
		part := bookrels[sheet.attr("id")]
		if part == "" {
			continue
		}
		root, err := z.node(part)
		if err != nil {
			return nil, nil, err
		}
		rels, err := z.rels(part)
		if err != nil {
			return nil, nil, err
		}
		for _, drawing := range root.descendants("drawing") {
			drawingPart := rels[drawing.attr("id")]
			if drawingPart == "" {
				continue
			}
			drawRoot, err := z.node(drawingPart)
			if err != nil {
				return nil, nil, err
			}
			drawrels, err := z.rels(drawingPart)
			if err != nil {
				return nil, nil, err
			}
			for _, anchor := range drawRoot.Children {
				origin := anchor.child("from")
				if origin.Name == "" {
					warnings = append(warnings, fmt.Sprintf("「%s」有绝对定位图片，无法确定所属行。", name))
					continue
				}
				row, col := IntText(origin.child("row").Text)+1, IntText(origin.child("col").Text)+1
				ext := anchor.child("ext")
				width, height := dimension(ext.attr("cx")), dimension(ext.attr("cy"))
				for _, blip := range anchor.descendants("blip") {
					if target := drawrels[blip.attr("embed")]; target != "" {
						addPicture(name, target, row, col, width, height)
					} else if blip.attr("link") != "" {
						warnings = append(warnings, fmt.Sprintf("「%s」第 %d 行为外链图片，文件中没有图片内容。", name, row))
					}
				}
			}
		}
		for _, cell := range root.child("sheetData").descendants("c") {
			formula := cell.child("f").Text
			if strings.Contains(strings.ToUpper(formula), "DISPIMG") {
				match := dispimgFormula.FindStringSubmatch(formula)
				target := ""
				if len(match) > 1 {
					target = cellpics[match[1]]
				}
				if target != "" {
					col, row, err := excelize.CellNameToCoordinates(cell.attr("r"))
					if err != nil {
						return nil, nil, err
					}
					addPicture(name, target, row, col, 0, 0)
				} else {
					warnings = append(warnings, fmt.Sprintf("「%s」%s 的单元格图片未包含可读取的图片文件。", name, cell.attr("r")))
				}
			} else if imageFormula.MatchString(formula) || cell.attr("vm") != "" {
				warnings = append(warnings, fmt.Sprintf("「%s」%s 使用外链或新版单元格图片；请核对，必要时转为浮动图片再导出。", name, cell.attr("r")))
			}
		}
	}
	unmapped := 0
	for name := range z.files {
		if strings.HasPrefix(name, "xl/media/") && !strings.HasSuffix(name, "/") && !mapped[name] {
			unmapped++
		}
	}
	if unmapped > 0 {
		warnings = append(warnings, fmt.Sprintf("原文件有 %d 个图片文件无法确定所属单元格，未自动放入任务行。请核对原表。", unmapped))
	}
	return pictures, UniqueStrings(warnings), ctx.Err()
}

func FilterWorkbook(ctx context.Context, filename, owner string, sheetNames []string, includeCompleted bool, options ...ReadOptions) (model.Document, error) {
	opts := readOptions(options)
	filename, err := filepath.Abs(filename)
	if err != nil {
		return model.Document{}, err
	}
	if !strings.EqualFold(filepath.Ext(filename), ".xlsx") {
		return model.Document{}, errors.New("请选择包含图片的 .xlsx 文件，旧版 .xls 请先另存为 .xlsx。")
	}
	if _, err := os.Stat(filename); err != nil {
		return model.Document{}, fmt.Errorf("源 Excel 无法打开，请重新导入：%w", err)
	}
	pictures, warnings, err := readExcelPictures(ctx, filename)
	if err != nil {
		return model.Document{}, err
	}
	book, err := excelize.OpenFile(filename, excelize.Options{UnzipSizeLimit: 256 * 1024 * 1024})
	if err != nil {
		return model.Document{}, err
	}
	defer book.Close()
	doc := model.Document{Owner: strings.TrimSpace(owner), SourcePath: filename, IncludeCompleted: includeCompleted, Title: strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename)), Warnings: warnings, Sheets: []model.TaskSheet{}}
	for _, name := range book.GetSheetList() {
		if err := ctx.Err(); err != nil {
			return model.Document{}, err
		}
		if len(sheetNames) > 0 {
			found := false
			for _, wanted := range sheetNames {
				if wanted == name {
					found = true
				}
			}
			if !found {
				continue
			}
		}
		values, err := book.GetRows(name)
		if err != nil {
			return model.Document{}, err
		}
		headerRow, ownerCol, width := -1, -1, 0
		for r, row := range values {
			width = max(width, len(row))
			if r >= 100 || headerRow >= 0 {
				continue
			}
			columns := []int{}
			for col, value := range row {
				if OwnerHeader(value) {
					columns = append(columns, col)
				}
			}
			if len(columns) > 1 && owner != "" {
				return model.Document{}, fmt.Errorf("「%s」同一表头有多列负责人，请保留一列后重试。", name)
			}
			if len(columns) == 1 {
				headerRow, ownerCol = r, columns[0]
			}
		}
		if owner == "" {
			grid := NewGrid()
			for r, row := range values {
				if r >= 100 {
					break
				}
				for col, value := range row {
					grid.Cells[CellPosition{r, col}] = value
				}
			}
			headerRow = discoverHeader(grid, false)
			ownerCol = -1
			if headerRow >= 0 {
				for col, h := range values[headerRow] {
					if OwnerHeader(h) {
						ownerCol = col
						break
					}
				}
			}
		}
		if headerRow < 0 {
			doc.Warnings = append(doc.Warnings, fmt.Sprintf("「%s」未找到“负责人 / 负责人员 / 责任人”表头，已跳过。", name))
			continue
		}
		picsByRow := map[int][]model.Picture{}
		for _, pic := range pictures[name] {
			width = max(width, pic.Column)
			picsByRow[pic.Row] = append(picsByRow[pic.Row], pic)
		}
		headers := make([]any, width)
		for col, value := range values[headerRow] {
			if value != "" {
				headers[col] = value
			}
		}
		ownerMerges := map[int]string{}
		merges, err := book.GetMergeCells(name)
		if err != nil {
			return model.Document{}, err
		}
		for _, merge := range merges {
			firstCol, firstRow, err := excelize.CellNameToCoordinates(merge.GetStartAxis())
			if err != nil {
				return model.Document{}, err
			}
			lastCol, lastRow, err := excelize.CellNameToCoordinates(merge.GetEndAxis())
			if err != nil {
				return model.Document{}, err
			}
			if ownerCol >= 0 && firstCol <= ownerCol+1 && lastCol >= ownerCol+1 {
				value, err := book.GetCellValue(name, merge.GetStartAxis())
				if err != nil {
					return model.Document{}, err
				}
				for r := max(firstRow, headerRow+2); r <= lastRow; r++ {
					ownerMerges[r] = value
				}
			}
		}
		filters, err := compileFilters(headers, name, opts.Conditions)
		if err != nil {
			return model.Document{}, err
		}
		sheet := model.TaskSheet{Name: name, HeaderRow: headerRow + 1, Headers: headers, Rows: []model.TaskRow{}}
		lastRow := len(values)
		for r := range picsByRow {
			lastRow = max(lastRow, r)
		}
		for row := headerRow + 1; row < lastRow; row++ {
			if err := ctx.Err(); err != nil {
				return model.Document{}, err
			}
			rowValues := make([]any, width)
			anyValue := false
			if row < len(values) {
				for col, value := range values[row] {
					if value != "" {
						rowValues[col] = value
						anyValue = true
					}
				}
			}
			if !anyValue && len(picsByRow[row+1]) == 0 {
				continue
			}
			sheet.Scanned++
			responsible := ""
			if ownerCol >= 0 {
				responsible = StringValue(rowValues[ownerCol])
			}
			if responsible == "" {
				responsible = ownerMerges[row+1]
			}
			if responsible == "" && ownerCol >= 0 && owner != "" {
				axis, _ := excelize.CoordinatesToCellName(ownerCol+1, row+1)
				formula, err := book.GetCellFormula(name, axis)
				if err != nil {
					return model.Document{}, err
				}
				if formula != "" {
					doc.Warnings = append(doc.Warnings, fmt.Sprintf("「%s」第 %d 行负责人公式没有缓存值，无法判断；请用 Excel 计算并保存后重试。", name, row+1))
				}
			}
			if owner != "" && !OwnerMatches(responsible, owner) {
				continue
			}
			if !includeCompleted && CompletedTask(headers, rowValues) {
				continue
			}
			if ownerCol >= 0 {
				rowValues[ownerCol] = responsible
			}
			for _, pic := range picsByRow[row+1] {
				switch StringValue(rowValues[pic.Column-1]) {
				case "#VALUE!", "#NAME?", "#N/A":
					rowValues[pic.Column-1] = nil
				}
			}
			if !matchesFilters(rowValues, filters) {
				continue
			}
			sheet.Rows = append(sheet.Rows, model.TaskRow{Number: row + 1, Values: rowValues, Pictures: picsByRow[row+1]})
		}
		doc.Sheets = append(doc.Sheets, sheet)
	}
	if len(doc.Sheets) == 0 {
		return model.Document{}, errors.New("没有找到“负责人 / 负责人员 / 责任人”列。请检查工作表与表头（自动检查前 100 行）。")
	}
	if err := validateFilterSheets(doc.Sheets, opts.Conditions); err != nil {
		return model.Document{}, err
	}
	doc.Warnings = UniqueStrings(doc.Warnings)
	return doc, nil
}
