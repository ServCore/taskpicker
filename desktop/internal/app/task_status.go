package app

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"taskpicker/internal/model"
	"taskpicker/internal/reader"
	"taskpicker/internal/storage"
)

type TaskNoteUpdate struct {
	Column int    `json:"column"`
	Value  string `json:"value"`
}

func pendingStatusCount(doc model.Document) int {
	count := 0
	for _, sheet := range doc.Sheets {
		for _, row := range sheet.Rows {
			if row.StatusEdit != nil {
				count++
			}
		}
	}
	return count
}

func StatusRow(doc *model.Document, sheetName string, rowNumber int) (*model.TaskSheet, *model.TaskRow, error) {
	for si := range doc.Sheets {
		sheet := &doc.Sheets[si]
		if sheet.Name != sheetName {
			continue
		}
		for ri := range sheet.Rows {
			if sheet.Rows[ri].Number == rowNumber {
				return sheet, &sheet.Rows[ri], nil
			}
		}
	}
	return nil, nil, errors.New("任务已变化，请重新打开任务列表。")
}

func persistStatusDocument(root string, doc model.Document) (model.Document, error) {
	return persistActivityDocument(root, doc)
}

func (a *App) startStatusOperation(timeout time.Duration) (context.Context, func(), error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.busy {
		return nil, nil, errors.New("当前操作尚未结束，请稍后重试。")
	}
	parent := a.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	a.busy = true
	a.cancel = cancel
	return ctx, func() { cancel(); a.mu.Lock(); a.busy = false; a.cancel = nil; a.mu.Unlock() }, nil
}

func (a *App) UpdateTaskStatus(key, sheetName string, rowNumber int, value string) (model.Document, error) {
	return a.UpdateTaskDetails(key, sheetName, rowNumber, value, nil)
}

func noteColumn(headers []any, col int) bool {
	return col >= 0 && col < len(headers) && strings.Contains(reader.Normalized(headers[col]), "备注")
}

func rowText(row *model.TaskRow, col int) string {
	if col < 0 || col >= len(row.Values) {
		return ""
	}
	return reader.StringValue(row.Values[col])
}

// UpdateTaskDetails persists status and all supplied notes in one local snapshot.
func (a *App) UpdateTaskDetails(key, sheetName string, rowNumber int, value string, notes []TaskNoteUpdate) (model.Document, error) {
	_, finish, err := a.startStatusOperation(15 * time.Second)
	if err != nil {
		return model.Document{}, err
	}
	defer finish()
	doc, err := ReadDocument(a.root, key)
	if err != nil {
		return model.Document{}, err
	}
	if doc.SourceURL == "" {
		return model.Document{}, errors.New("Excel 导入任务暂不支持状态与备注回写，请打开源 Excel 修改。")
	}

	sheet, row, err := StatusRow(&doc, sheetName, rowNumber)
	if err != nil {
		return model.Document{}, err
	}
	before := *row
	before.Values = append([]any(nil), row.Values...)
	col := reader.StatusColumn(sheet.Headers)
	if col < 0 || col >= len(sheet.Headers) {
		return model.Document{}, errors.New("这份表格没有可编辑的状态列。")
	}
	current := rowText(row, col)
	value = strings.TrimSpace(value)
	if (value == "" && !(len(notes) > 0 && strings.TrimSpace(current) == "")) || utf8.RuneCountInString(value) > 80 || strings.ContainsAny(value, "\r\n") || strings.HasPrefix(value, "=") {
		return model.Document{}, errors.New("请选择有效的任务状态。")
	}
	seen := map[int]bool{}
	for _, note := range notes {
		if !noteColumn(sheet.Headers, note.Column) || seen[note.Column] {
			return model.Document{}, errors.New("备注列已变化或重复，请重新同步文档。")
		}
		if !utf8.ValidString(note.Value) || utf8.RuneCountInString(note.Value) > 10000 {
			return model.Document{}, errors.New("备注最多支持 10000 个字符。")
		}
		seen[note.Column] = true
	}
	if row.StatusEdit == nil {
		row.StatusEdit = &model.StatusEdit{Original: current}
	}
	for len(row.Values) <= col {
		row.Values = append(row.Values, "")
	}
	row.Values[col] = value
	for _, note := range notes {
		original := rowText(row, note.Column)
		index := -1
		for i, edit := range row.StatusEdit.Notes {
			if edit.Column == note.Column {
				original = edit.Original
				index = i
				break
			}
		}
		desired := strings.ReplaceAll(note.Value, "\r\n", "\n")
		if index >= 0 {
			row.StatusEdit.Notes = append(row.StatusEdit.Notes[:index], row.StatusEdit.Notes[index+1:]...)
		}
		if desired != original {
			row.StatusEdit.Notes = append(row.StatusEdit.Notes, model.NoteEdit{Column: note.Column, Header: reader.StringValue(sheet.Headers[note.Column]), Original: original})
		}
		for len(row.Values) <= note.Column {
			row.Values = append(row.Values, "")
		}
		row.Values[note.Column] = desired
	}
	row.StatusEdit.SavedAt = time.Now().Format(time.RFC3339)
	row.StatusEdit.Error = ""
	row.StatusSyncedAt = ""
	if value == row.StatusEdit.Original && len(row.StatusEdit.Notes) == 0 {
		row.StatusEdit = nil
	}
	changes := rowActivityChanges(*sheet, before, *row)
	if len(changes) == 0 {
		return persistStatusDocument(a.root, doc)
	}
	activity := newTaskActivity(doc, *sheet, *row, "edit")
	activity.Changes = changes
	return persistActivityDocument(a.root, doc, activity)
}

func (a *App) RevertTaskStatus(key, sheetName string, rowNumber int) (model.Document, error) {
	_, finish, err := a.startStatusOperation(15 * time.Second)
	if err != nil {
		return model.Document{}, err
	}
	defer finish()
	doc, err := ReadDocument(a.root, key)
	if err != nil {
		return model.Document{}, err
	}
	sheet, row, err := StatusRow(&doc, sheetName, rowNumber)
	if err != nil {
		return model.Document{}, err
	}
	if row.StatusEdit == nil {
		return doc, nil
	}
	before := *row
	before.Values = append([]any(nil), row.Values...)
	col := reader.StatusColumn(sheet.Headers)
	if col < 0 || col >= len(row.Values) {
		return model.Document{}, errors.New("状态列已变化，无法撤销。")
	}
	for _, note := range row.StatusEdit.Notes {
		if !noteColumn(sheet.Headers, note.Column) || reader.StringValue(sheet.Headers[note.Column]) != note.Header {
			return model.Document{}, errors.New("备注列已变化，无法撤销。")
		}
		for len(row.Values) <= note.Column {
			row.Values = append(row.Values, "")
		}
		row.Values[note.Column] = note.Original
	}
	row.Values[col] = row.StatusEdit.Original
	row.StatusEdit = nil
	activity := newTaskActivity(doc, *sheet, *row, "revert")
	activity.Changes = rowActivityChanges(*sheet, before, *row)
	return persistActivityDocument(a.root, doc, activity)
}

func (a *App) PushTaskStatus(key, sheetName string, rowNumber int) (model.Document, error) {
	ctx, finish, err := a.startStatusOperation(90 * time.Second)
	if err != nil {
		return model.Document{}, err
	}
	defer finish()
	doc, err := ReadDocument(a.root, key)
	if err != nil {
		return model.Document{}, err
	}
	sheet, row, err := StatusRow(&doc, sheetName, rowNumber)
	if err != nil {
		return model.Document{}, err
	}
	if row.StatusEdit == nil {
		return doc, nil
	}
	client, err := newDocsRemoteClient(ctx, a.root)
	if err == nil {
		err = pushStatus(ctx, client, &doc, sheet, row)
	}
	if err != nil {
		row.StatusEdit.Error = err.Error()
		activity := newTaskActivity(doc, *sheet, *row, "sync_failed")
		activity.Note = "远程同步未确认，可能部分写入；本地修改已保留，可重试。"
		if _, saveErr := persistActivityDocument(a.root, doc, activity); saveErr != nil {
			return model.Document{}, fmt.Errorf("%v；保存失败信息时出错：%w", err, saveErr)
		}
		return doc, nil // Return the saved row with its retryable error, never discard the local edit.
	}
	row.StatusEdit = nil
	row.StatusSyncedAt = time.Now().Format(time.RFC3339)
	activity := newTaskActivity(doc, *sheet, *row, "synced")
	activity.Note = "状态与备注已回写远程并回读确认。"
	return persistActivityDocument(a.root, doc, activity)
}

type remoteStatusTarget struct {
	read    func(context.Context) (string, error)
	write   func(context.Context, string) error
	choices []string
}

func pushStatus(ctx context.Context, client remoteCaller, doc *model.Document, sheet *model.TaskSheet, row *model.TaskRow) error {
	if row.StatusEdit == nil {
		return nil
	}
	type change struct {
		col                      int
		label, original, desired string
		target                   *remoteStatusTarget
		pending                  bool
	}
	var changes []change
	// Confirm notes first; leave the status pending until notes are confirmed too.
	for _, note := range row.StatusEdit.Notes {
		if !noteColumn(sheet.Headers, note.Column) || reader.StringValue(sheet.Headers[note.Column]) != note.Header {
			return errors.New("备注列已变化，请先核对原表。")
		}
		changes = append(changes, change{col: note.Column, label: strings.TrimSpace(note.Header), original: note.Original, desired: rowText(row, note.Column)})
	}
	col := reader.StatusColumn(sheet.Headers)
	if col < 0 || col >= len(row.Values) {
		return errors.New("任务没有状态字段。")
	}
	if rowText(row, col) != row.StatusEdit.Original {
		changes = append(changes, change{col: col, label: "状态", original: row.StatusEdit.Original, desired: rowText(row, col)})
	}
	check := func(item *change) error {
		current, err := item.target.read(ctx)
		if err != nil {
			return err
		}
		item.pending = current != item.desired
		if item.pending && current != item.original {
			return fmt.Errorf("远程%s已被修改。本地修改已保留，请先核对原表，再撤销本地修改并重新同步文档。", item.label)
		}
		return nil
	}
	// Preflight every field before writing anything, including note conflicts.
	for i := range changes {
		item := &changes[i]
		target, err := resolveFieldTarget(ctx, client, *doc, *sheet, *row, item.col)
		if err != nil {
			return fmt.Errorf("%s：%w", item.label, err)
		}
		item.target = target
		if err = check(item); err != nil {
			return fmt.Errorf("%s：%w", item.label, err)
		}
		if item.pending && len(target.choices) > 0 {
			found := false
			for _, choice := range target.choices {
				if choice == item.desired {
					found = true
				}
			}
			if !found {
				return errors.New("所选状态不在远程字段的可选项中，请重新同步文档。")
			}
		}
	}
	for i := range changes {
		item := &changes[i]
		if !item.pending {
			continue
		}
		// Recheck just before each write; retries also accept already-applied values.
		if err := check(item); err != nil {
			return fmt.Errorf("%s：%w", item.label, err)
		}
		if !item.pending {
			continue
		}
		if err := item.target.write(ctx, item.desired); err != nil {
			return fmt.Errorf("%s：%w", item.label, err)
		}
		current, err := item.target.read(ctx)
		if err != nil {
			return fmt.Errorf("%s已提交，但尚未确认远程结果；请重试同步进行核对：%w", item.label, err)
		}
		if current != item.desired {
			return fmt.Errorf("%s的远程回读结果与本地修改不一致，修改仍保留为待同步。", item.label)
		}
	}
	return nil
}

func identityColumns(headers []any) ([]int, error) {
	choose := func(names []string) int {
		for _, name := range names {
			for col, h := range headers {
				if reader.Normalized(h) == name {
					return col
				}
			}
		}
		return -1
	}
	title := reader.TitleColumn(headers)
	owner := choose([]string{"负责人", "负责人员", "责任人"})
	if title < 0 {
		return nil, errors.New("缺少可核对的任务内容字段，无法可靠定位原任务。")
	}
	cols := []int{title}
	if owner >= 0 {
		cols = append(cols, owner)
	}
	context := choose([]string{"三级功能", "功能", "二级功能", "模块", "项目"})
	if context >= 0 && context != title {
		cols = append(cols, context)
	}
	return cols, nil
}

func sameTaskValue(a, b any) bool {
	return strings.TrimSpace(strings.ReplaceAll(reader.StringValue(a), "\r\n", "\n")) == strings.TrimSpace(strings.ReplaceAll(reader.StringValue(b), "\r\n", "\n"))
}

func resolveStatusTarget(ctx context.Context, client remoteCaller, doc model.Document, sheet model.TaskSheet, row model.TaskRow) (*remoteStatusTarget, error) {
	return resolveFieldTarget(ctx, client, doc, sheet, row, reader.StatusColumn(sheet.Headers))
}

func resolveFieldTarget(ctx context.Context, client remoteCaller, doc model.Document, sheet model.TaskSheet, row model.TaskRow, fieldCol int) (*remoteStatusTarget, error) {
	link, err := storage.ValidateURL(doc.SourceURL)
	if err != nil {
		return nil, err
	}
	parsed, _ := url.Parse(link)
	fileID := strings.TrimPrefix(parsed.Path[strings.LastIndex(parsed.Path, "/"):], "/")
	meta, err := client.Call(ctx, "sheet.get_sheet_info", map[string]any{"file_id": fileID})
	if err != nil {
		return nil, err
	}
	tab := parsed.Query().Get("tab")
	var selected map[string]any
	for _, value := range reader.Array(meta["sheets"]) {
		candidate := reader.Object(value)
		if (tab != "" && reader.StringValue(candidate["sheet_id"]) == tab) || (tab == "" && reader.StringValue(candidate["sheet_name"]) == sheet.Name) {
			if selected != nil {
				return nil, errors.New("原表工作表不唯一，无法回写。")
			}
			selected = candidate
		}
	}
	if selected == nil || reader.StringValue(selected["sheet_name"]) != sheet.Name {
		return nil, errors.New("原工作表已变化，请先核对原表。")
	}
	args := map[string]any{"file_id": fileID, "sheet_id": reader.StringValue(selected["sheet_id"])}
	cols, err := identityColumns(sheet.Headers)
	if err != nil {
		return nil, err
	}
	statusCol := fieldCol
	if statusCol < 0 || statusCol >= len(sheet.Headers) {
		return nil, errors.New("任务字段已变化。")
	}
	if reader.StringValue(selected["sheet_type"]) == "smartsheet" {
		return smartStatusTarget(ctx, client, args, parsed.Query().Get("viewId"), sheet, row, cols, statusCol)
	}
	if reader.StringValue(selected["sheet_type"]) != "worksheet" {
		return nil, errors.New("这类工作表暂不支持回写。")
	}
	return regularStatusTarget(ctx, client, args, sheet, row, cols, statusCol)
}

func copyArgs(args map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range args {
		out[k] = v
	}
	return out
}

func sheetCells(ctx context.Context, client remoteCaller, args map[string]any, row, width int) (map[int]map[string]any, error) {
	query := copyArgs(args)
	query["start_row"] = row
	query["end_row"] = row + 1
	query["start_col"] = 0
	query["end_col"] = width
	query["return_csv"] = false
	query["include_formula"] = true
	result, err := client.Call(ctx, "sheet.get_cell_data", query)
	if err != nil {
		return nil, err
	}
	cells := map[int]map[string]any{}
	for _, value := range reader.Array(result["cells"]) {
		cell := reader.Object(value)
		if int(reader.Number(cell["row"])) == row {
			cells[int(reader.Number(cell["col"]))] = cell
		}
	}
	return cells, nil
}

func regularStatusTarget(ctx context.Context, client remoteCaller, args map[string]any, sheet model.TaskSheet, row model.TaskRow, cols []int, statusCol int) (*remoteStatusTarget, error) {
	headers, err := sheetCells(ctx, client, args, sheet.HeaderRow-1, len(sheet.Headers))
	if err != nil {
		return nil, err
	}
	for _, col := range append(cols, statusCol) {
		if !sameTaskValue(headers[col]["string_value"], sheet.Headers[col]) {
			return nil, errors.New("远程表头或列顺序已变化，请核对原表后重新读取。")
		}
	}
	read := func(ctx context.Context) (string, error) {
		cells, err := sheetCells(ctx, client, args, row.Number-1, len(sheet.Headers))
		if err != nil {
			return "", err
		}
		for _, col := range cols {
			if col >= len(row.Values) || !sameTaskValue(cells[col]["string_value"], row.Values[col]) {
				return "", errors.New("原任务的行位置、内容或负责人已变化，已停止回写，避免修改其他任务。")
			}
		}
		if reader.StringValue(cells[statusCol]["formula"]) != "" {
			return "", errors.New("字段由公式生成，不能直接回写。")
		}
		return reader.StringValue(cells[statusCol]["string_value"]), nil
	}
	write := func(ctx context.Context, value string) error {
		query := copyArgs(args)
		query["row"] = row.Number - 1
		query["col"] = statusCol
		query["value_type"] = "STRING"
		query["string_value"] = value
		_, err := client.Call(ctx, "sheet.set_cell_value", query)
		return err
	}
	return &remoteStatusTarget{read: read, write: write}, nil
}

func smartValue(field map[string]any) string {
	for _, key := range []string{"text_value", "option_value", "url_value"} {
		if v, ok := field[key]; ok {
			var parts []string
			for _, item := range reader.Array(reader.Object(v)["items"]) {
				parts = append(parts, reader.StringValue(reader.Object(item)["text"]))
			}
			sep := ""
			if key == "option_value" {
				sep = "、"
			}
			return strings.Join(parts, sep)
		}
	}
	return reader.StringValue(field["string_value"])
}

func smartRecordValues(record map[string]any) map[string]string {
	values := map[string]string{}
	for _, item := range reader.Array(record["field_values"]) {
		field := reader.Object(item)
		values[reader.StringValue(field["field"])] = smartValue(field)
	}
	return values
}

func smartStatusTarget(ctx context.Context, client remoteCaller, args map[string]any, viewID string, sheet model.TaskSheet, row model.TaskRow, cols []int, statusCol int) (*remoteStatusTarget, error) {
	label := reader.StringValue(sheet.Headers[statusCol])
	query := copyArgs(args)
	query["field_titles"] = []string{label}
	query["limit"] = 100
	if viewID != "" {
		query["view_id"] = viewID
	}
	fields, err := client.Call(ctx, "smartsheet.list_fields", query)
	if err != nil {
		return nil, err
	}
	var field map[string]any
	for _, v := range reader.Array(fields["fields"]) {
		f := reader.Object(v)
		if reader.StringValue(f["field_title"]) == label {
			if field != nil {
				return nil, errors.New("远程字段不唯一，无法回写。")
			}
			field = f
		}
	}
	if field == nil {
		return nil, errors.New("远程字段已被删除或改名。")
	}
	kind := reader.StringValue(field["field_type"])
	if noteColumn(sheet.Headers, statusCol) && kind != "text" {
		return nil, errors.New("备注字段不是可编辑文本，无法回写。")
	}
	if kind != "singleSelect" && kind != "select" && kind != "text" {
		return nil, errors.New("远程字段类型暂不支持回写。")
	}
	var choices []string
	for _, key := range []string{"property_single_select", "property_select"} {
		for _, v := range reader.Array(reader.Object(field[key])["options"]) {
			choices = append(choices, reader.StringValue(reader.Object(v)["text"]))
		}
	}
	labels := []string{label}
	for _, col := range cols {
		labels = append(labels, reader.StringValue(sheet.Headers[col]))
	}
	recordsQuery := copyArgs(args)
	recordsQuery["field_titles"] = labels
	recordsQuery["limit"] = 100
	if viewID != "" {
		recordsQuery["view_id"] = viewID
	}
	matches := func(record map[string]any) bool {
		values := smartRecordValues(record)
		for _, col := range cols {
			if col >= len(row.Values) || !sameTaskValue(values[reader.StringValue(sheet.Headers[col])], row.Values[col]) {
				return false
			}
		}
		return true
	}
	recordID := row.RecordID
	if recordID == "" {
		for offset := 0; ; {
			recordsQuery["offset"] = offset
			page, err := client.Call(ctx, "smartsheet.list_records", recordsQuery)
			if err != nil {
				return nil, err
			}
			for _, v := range reader.Array(page["records"]) {
				record := reader.Object(v)
				if matches(record) {
					if recordID != "" {
						return nil, errors.New("远程存在内容相同的多条任务，无法唯一定位；请重新同步文档。")
					}
					recordID = reader.StringValue(record["record_id"])
				}
			}
			if page["has_more"] != true {
				break
			}
			next := int(reader.Number(page["next"]))
			if next <= offset || next > 100000 {
				return nil, errors.New("智能表记录分页异常。")
			}
			offset = next
		}
	}
	if recordID == "" {
		return nil, errors.New("原任务内容或负责人已变化，无法定位远程记录。")
	}
	read := func(ctx context.Context) (string, error) {
		q := copyArgs(recordsQuery)
		q["record_ids"] = []string{recordID}
		delete(q, "offset")
		result, err := client.Call(ctx, "smartsheet.list_records", q)
		if err != nil {
			return "", err
		}
		records := reader.Array(result["records"])
		if len(records) != 1 || reader.StringValue(reader.Object(records[0])["record_id"]) != recordID || !matches(reader.Object(records[0])) {
			return "", errors.New("原任务记录、内容或负责人已变化，已停止回写。")
		}
		return smartRecordValues(reader.Object(records[0]))[label], nil
	}
	write := func(ctx context.Context, value string) error {
		v := map[string]any{"field": label}
		if kind == "text" {
			items := []any{}
			if value != "" {
				items = append(items, map[string]any{"text": value, "type": "text"})
			}
			v["text_value"] = map[string]any{"items": items}
		} else {
			v["option_value"] = map[string]any{"items": []any{map[string]any{"text": value}}}
		}
		q := copyArgs(args)
		q["records"] = []any{map[string]any{"record_id": recordID, "field_values": []any{v}}}
		_, err := client.Call(ctx, "smartsheet.update_records", q)
		return err
	}
	return &remoteStatusTarget{read: read, write: write, choices: choices}, nil
}
