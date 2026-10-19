package reader

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
)

type wireValue struct {
	Integer uint64
	Bytes   []byte
}

type wireFields map[int][]wireValue

func WireDecode(raw []byte) (wireFields, error) {
	fields := wireFields{}
	for pos := 0; pos < len(raw); {
		tag, n := binary.Uvarint(raw[pos:])
		if n <= 0 {
			return nil, errors.New("表格数据中的整数不完整。")
		}
		pos += n
		id, kind := int(tag>>3), tag&7
		if id == 0 {
			return nil, errors.New("表格数据中的字段编号异常。")
		}
		var v wireValue
		switch kind {
		case 0:
			value, n := binary.Uvarint(raw[pos:])
			if n <= 0 {
				return nil, errors.New("表格数据中的整数格式异常。")
			}
			pos += n
			v.Integer = value
		case 1, 5:
			size := 8
			if kind == 5 {
				size = 4
			}
			if size > len(raw)-pos {
				return nil, errors.New("表格数据不完整，请重新同步。")
			}
			v.Bytes = raw[pos : pos+size]
			pos += size
		case 2:
			size, n := binary.Uvarint(raw[pos:])
			if n <= 0 {
				return nil, errors.New("表格数据中的长度不完整。")
			}
			pos += n
			if size > uint64(len(raw)-pos) {
				return nil, errors.New("表格数据不完整，请重新同步。")
			}
			v.Bytes = raw[pos : pos+int(size)]
			pos += int(size)
		default:
			return nil, errors.New("暂不支持此表格的二进制编码。")
		}
		fields[id] = append(fields[id], v)
	}
	return fields, nil
}

func (w wireFields) first(id int) wireValue {
	if len(w[id]) > 0 {
		return w[id][0]
	}
	return wireValue{}
}

func wrappedValue(raw []byte) (wireValue, error) {
	w, err := WireDecode(raw)
	if err != nil {
		return wireValue{}, err
	}
	return w.first(1), nil
}

func DecodeGrid(encoded, tab string) (*GridData, error) {
	compressed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("表格数据编码异常：%w", err)
	}
	zr, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	raw, err := io.ReadAll(io.LimitReader(zr, 64*1024*1024+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 64*1024*1024 {
		return nil, errors.New("单个表格数据块超过 64 MB，请缩小表格范围。")
	}
	root, err := WireDecode(raw)
	if err != nil {
		return nil, err
	}
	container, err := WireDecode(root.first(1).Bytes)
	if err != nil {
		return nil, err
	}
	out := NewGrid()
	for _, operation := range container[5] {
		op, err := WireDecode(operation.Bytes)
		if err != nil {
			return nil, err
		}
		for _, encodedSheet := range op[19] {
			sheet, err := WireDecode(encodedSheet.Bytes)
			if err != nil {
				return nil, err
			}
			bounds, err := WireDecode(sheet.first(3).Bytes)
			if err != nil {
				return nil, err
			}
			if string(bounds.first(1).Bytes) != tab {
				continue
			}
			out.Ranges = append(out.Ranges, gridRange{int(bounds.first(2).Integer), int(bounds.first(4).Integer), int(bounds.first(3).Integer), int(bounds.first(5).Integer)})
			pool, err := WireDecode(sheet.first(5).Bytes)
			if err != nil {
				return nil, err
			}
			var texts, rich []string
			for _, v := range pool[1] {
				value, err := wrappedValue(v.Bytes)
				if err != nil {
					return nil, err
				}
				texts = append(texts, string(value.Bytes))
			}
			for _, v := range pool[2] {
				item, err := WireDecode(v.Bytes)
				if err != nil {
					return nil, err
				}
				var builder strings.Builder
				for _, run := range item[3] {
					fields, err := WireDecode(run.Bytes)
					if err != nil {
						return nil, err
					}
					value, err := wrappedValue(fields.first(3).Bytes)
					if err != nil {
						return nil, err
					}
					builder.Write(value.Bytes)
				}
				rich = append(rich, builder.String())
			}
			numbers := make([]float64, 129)
			for i := range numbers {
				numbers[i] = float64(i)
			}
			for _, v := range pool[3] {
				value, err := wrappedValue(v.Bytes)
				if err != nil {
					return nil, err
				}
				if len(value.Bytes) != 8 {
					return nil, errors.New("数值数据池格式异常。")
				}
				numbers = append(numbers, math.Float64frombits(binary.LittleEndian.Uint64(value.Bytes)))
			}
			for _, encodedCell := range sheet[6] {
				cell, err := WireDecode(encodedCell.Bytes)
				if err != nil {
					return nil, err
				}
				row, col := int(cell.first(1).Integer), int(cell.first(2).Integer)
				if row < 0 || col < 0 || row > 10_000_000 || col > 100_000 {
					return nil, errors.New("单元格坐标异常。")
				}
				data, err := WireDecode(cell.first(3).Bytes)
				if err != nil {
					return nil, err
				}
				indexValue, err := wrappedValue(data.first(2).Bytes)
				if err != nil {
					return nil, err
				}
				index := int(indexValue.Integer)
				kind := data.first(1).Integer
				var value any
				switch kind {
				case 4:
					if index < 0 || index >= len(texts) {
						return nil, errors.New("共享文本数据池不完整，请重新同步。")
					}
					value = texts[index]
				case 2:
					if index < 0 || index >= len(numbers) {
						return nil, errors.New("共享数值数据池不完整，请重新同步。")
					}
					value = numbers[index]
				case 6:
					if index < 0 || index >= len(rich) {
						return nil, errors.New("共享富文本数据池不完整，请重新同步。")
					}
					value = rich[index]
				case 1:
					value = index != 0
				case 0:
				default:
					out.Warnings = append(out.Warnings, fmt.Sprintf("原第 %d 行、第 %d 列的单元格类型 %d 暂不支持。", row+1, col+1, kind))
				}
				extra, err := wrappedValue(data.first(17).Bytes)
				if err != nil {
					return nil, err
				}
				if len(extra.Bytes) > 0 {
					switch data.first(16).Integer {
					case 1:
						var people [][]any
						if err := json.Unmarshal(extra.Bytes, &people); err != nil {
							return nil, err
						}
						names := []string{}
						for _, person := range people {
							if len(person) < 2 {
								return nil, errors.New("人员字段数据不完整。")
							}
							names = append(names, StringValue(person[1]))
						}
						value = strings.Join(names, "、")
					case 5:
						var picture []any
						if err := json.Unmarshal(extra.Bytes, &picture); err != nil {
							return nil, err
						}
						if len(picture) > 0 && StringValue(picture[0]) != "" {
							pic := RemoteImage{Row: row, Col: col, URL: StringValue(picture[0])}
							if len(picture) > 1 {
								pic.Width = Number(picture[1])
							}
							if len(picture) > 2 {
								pic.Height = Number(picture[2])
							}
							out.Images[CellPosition{row, col}] = []RemoteImage{pic}
						} else {
							out.Warnings = append(out.Warnings, fmt.Sprintf("原第 %d 行图片格式有变化，请核对。", row+1))
						}
					}
				}
				out.Cells[CellPosition{row, col}] = value
			}
		}
	}
	return out, nil
}
