package app

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"taskpicker/internal/reader"
)

func excelFixture(t *testing.T) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "任务.xlsx")
	book := excelize.NewFile()
	defer book.Close()
	if err := book.SetSheetName("Sheet1", "问题清单"); err != nil {
		t.Fatal(err)
	}
	rows := [][]any{{"测试项目说明"}, {"任务", "负责 人", "截图", "备注"}, {"单人任务", "王小明", nil, "=literal text"}, {"多人任务", "李小华、王小明"}, {"其他人", "王小明双"}, {"合并任务 A", "王小明"}, {"合并任务 B"}}
	for i, row := range rows {
		axis, _ := excelize.CoordinatesToCellName(1, i+1)
		if err := book.SetSheetRow("问题清单", axis, &row); err != nil {
			t.Fatal(err)
		}
	}
	if err := book.MergeCell("问题清单", "B6", "B7"); err != nil {
		t.Fatal(err)
	}
	for i, cell := range []string{"C3", "C4", "C5", "C4"} {
		raw := testPNG(t, color.NRGBA{R: uint8(40 * i), B: uint8(255 - 40*i), A: 255})
		if err := book.AddPictureFromBytes("问题清单", cell, &excelize.Picture{Extension: ".png", File: raw}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := book.NewSheet("智能表格"); err != nil {
		t.Fatal(err)
	}
	for i, row := range [][]any{{"项目", "负责人员"}, {"第二页任务", "李小华, 王小明"}} {
		axis, _ := excelize.CoordinatesToCellName(1, i+1)
		if err := book.SetSheetRow("智能表格", axis, &row); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := book.NewSheet("无负责人"); err != nil {
		t.Fatal(err)
	}
	if err := book.SaveAs(filename); err != nil {
		t.Fatal(err)
	}
	return filename
}

func TestExcelFilterMergedOwnersAndFloatingPictures(t *testing.T) {
	filename := excelFixture(t)
	before, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := reader.FilterWorkbook(context.Background(), filename, "王小明", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Sheets) != 2 || len(doc.Sheets[0].Rows) != 4 || len(doc.Sheets[1].Rows) != 1 {
		t.Fatalf("bad workbook selection: %+v", doc.Sheets)
	}
	rows := doc.Sheets[0].Rows
	numbers := []int{}
	pictures := 0
	for _, row := range rows {
		numbers = append(numbers, row.Number)
		pictures += len(row.Pictures)
	}
	if !reflect.DeepEqual(numbers, []int{3, 4, 6, 7}) || pictures != 3 || rows[3].Values[1] != "王小明" {
		t.Fatalf("merged owners or pictures changed: %+v, pictures %d", numbers, pictures)
	}
	if rows[0].Values[3] != "=literal text" {
		t.Fatal("literal text was evaluated")
	}
	if len(doc.Warnings) != 1 {
		t.Fatalf("missing unsupported-sheet warning: %+v", doc.Warnings)
	}
	after, _ := os.ReadFile(filename)
	if sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("import modified the source workbook")
	}
	selected, err := reader.FilterWorkbook(context.Background(), filename, "王小明", []string{"智能表格"}, false)
	if err != nil || len(selected.Sheets) != 1 || len(selected.Sheets[0].Rows) != 1 {
		t.Fatalf("sheet selection failed: %v", err)
	}
}

func TestExcelHeaderAliasEmptySelectionAndMissingHeader(t *testing.T) {
	filename := excelFixture(t)
	book, err := excelize.OpenFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellStr("问题清单", "B2", "责任人"); err != nil {
		t.Fatal(err)
	}
	if err := book.Save(); err != nil {
		t.Fatal(err)
	}
	book.Close()
	doc, err := reader.FilterWorkbook(context.Background(), filename, "王小明", nil, false)
	if err != nil || len(doc.Sheets[0].Rows) != 4 {
		t.Fatal("responsibility header alias failed", err)
	}
	if _, err := reader.FilterWorkbook(context.Background(), filename, "王小明", []string{"无负责人"}, false); err == nil {
		t.Fatal("accepted sheet without an owner header")
	}
	empty, err := reader.FilterWorkbook(context.Background(), filename, "不存在的人", nil, false)
	if err != nil || len(empty.Sheets[0].Rows) != 0 {
		t.Fatal("no-match selection failed", err)
	}
	all, err := reader.FilterWorkbook(context.Background(), filename, "", nil, false)
	if err != nil || len(all.Sheets[0].Rows) != 5 {
		t.Fatalf("optional owner failed: %+v %v", all.Sheets, err)
	}
}

func appendZipParts(t *testing.T, filename string, parts map[string][]byte) {
	t.Helper()
	original, err := zip.OpenReader(filename)
	if err != nil {
		t.Fatal(err)
	}
	var target bytes.Buffer
	writer := zip.NewWriter(&target)
	for _, file := range original.File {
		if _, replace := parts[file.Name]; replace {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		destination, err := writer.CreateHeader(&file.FileHeader)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(destination, reader); err != nil {
			t.Fatal(err)
		}
		reader.Close()
	}
	original.Close()
	for name, raw := range parts {
		destination, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := destination.Write(raw); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, target.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestExcelDispimgAndUnmappedImageWarnings(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "单元格图片.xlsx")
	book := excelize.NewFile()
	for i, row := range [][]any{{"任务", "负责人", "图片"}, {"带图片任务", "王小明"}} {
		axis, _ := excelize.CoordinatesToCellName(1, i+1)
		if err := book.SetSheetRow("Sheet1", axis, &row); err != nil {
			t.Fatal(err)
		}
	}
	if err := book.SetCellFormula("Sheet1", "C2", `_xlfn.DISPIMG("ID_TEST",1)`); err != nil {
		t.Fatal(err)
	}
	if err := book.SaveAs(filename); err != nil {
		t.Fatal(err)
	}
	book.Close()
	appendZipParts(t, filename, map[string][]byte{
		"xl/cellimages.xml":            []byte(`<etc:cellImages xmlns:etc="http://www.wps.cn/officeDocument/2017/etCustomData" xmlns:xdr="http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><etc:cellImage><xdr:pic><xdr:nvPicPr><xdr:cNvPr id="1" name="ID_TEST"/></xdr:nvPicPr><xdr:blipFill><a:blip r:embed="rId1"/></xdr:blipFill></xdr:pic></etc:cellImage></etc:cellImages>`),
		"xl/_rels/cellimages.xml.rels": []byte(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/picture.png"/></Relationships>`),
		"xl/media/picture.png":         testPNG(t, color.NRGBA{R: 128, B: 128, A: 255}),
	})
	doc, err := reader.FilterWorkbook(context.Background(), filename, "王小明", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Sheets[0].Rows) != 1 || len(doc.Sheets[0].Rows[0].Pictures) != 1 || len(doc.Warnings) != 0 {
		t.Fatalf("lost DISPIMG picture: %+v", doc)
	}
	pic := doc.Sheets[0].Rows[0].Pictures[0]
	if pic.Row != 2 || pic.Column != 3 {
		t.Fatal("cell image attached to wrong row")
	}
	appendZipParts(t, filename, map[string][]byte{"xl/media/orphan.png": testPNG(t, color.NRGBA{R: 255, A: 255})})
	doc, err = reader.FilterWorkbook(context.Background(), filename, "王小明", nil, false)
	if err != nil || !strings.Contains(strings.Join(doc.Warnings, ""), "无法确定") {
		t.Fatalf("missing orphan image warning: %v %+v", err, doc.Warnings)
	}
}
