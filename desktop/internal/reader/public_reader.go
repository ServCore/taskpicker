package reader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"taskpicker/internal/model"
	"taskpicker/internal/storage"
)

var ErrNeedsLogin = errors.New("腾讯文档需要登录有查看权限的账号。")

var metadataScript = regexp.MustCompile(`<script[^>]+src=["']([^"']*dop-api/opendoc[^"']+)["']`)

type publicSession struct {
	client           *http.Client
	source, endpoint string
}

func NewHTTPClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, Timeout: 25 * time.Second}
}

func LimitedRead(reader io.Reader, limit int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, errors.New("响应数据过大，请缩小表格或图片范围。")
	}
	return raw, nil
}

func fetch(ctx context.Context, client *http.Client, link, referer string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", referer)
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return nil, ErrNeedsLogin
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("读取失败，HTTP %d。", response.StatusCode)
	}
	return LimitedRead(response.Body, limit)
}

func newPublicSession(ctx context.Context, link string, client *http.Client) (*publicSession, error) {
	if client == nil {
		client = NewHTTPClient()
	}
	raw, err := fetch(ctx, client, link, link, 32*1024*1024)
	if err != nil {
		return nil, err
	}
	match := metadataScript.FindSubmatch(raw)
	if len(match) != 2 {
		return nil, ErrNeedsLogin
	}
	base, _ := url.Parse(link)
	relative, err := url.Parse(html.UnescapeString(string(match[1])))
	if err != nil {
		return nil, err
	}
	endpoint := base.ResolveReference(relative)
	if endpoint.Scheme != "https" || endpoint.Hostname() != "docs.qq.com" || endpoint.User != nil {
		return nil, errors.New("文档返回了无法识别的数据地址。")
	}
	return &publicSession{client: client, source: link, endpoint: endpoint.String()}, nil
}

func decodeMetadata(raw []byte) (map[string]any, error) {
	trimmed := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(trimmed, "{") {
		start, end := strings.IndexByte(trimmed, '('), strings.LastIndexByte(trimmed, ')')
		if start < 0 || end <= start {
			return nil, errors.New("腾讯文档返回的数据格式异常。")
		}
		trimmed = trimmed[start+1 : end]
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(trimmed), &data); err != nil {
		return nil, err
	}
	return Object(data["clientVars"]), nil
}

func (s *publicSession) load(ctx context.Context, parameters map[string]int) (map[string]any, error) {
	u, _ := url.Parse(s.endpoint)
	query := u.Query()
	for key, value := range parameters {
		query.Set(key, strconv.Itoa(value))
	}
	u.RawQuery = query.Encode()
	raw, err := fetch(ctx, s.client, u.String(), s.source, 32*1024*1024)
	if err != nil {
		return nil, err
	}
	client, err := decodeMetadata(raw)
	if err != nil {
		return nil, err
	}
	privilege := Object(client["privilegeAttribute"])["can_read"]
	if Number(privilege) == 0 && privilege != true {
		return nil, ErrNeedsLogin
	}
	return client, nil
}

func clientTexts(client map[string]any) []any {
	return Array(Object(Object(client["collab_client_vars"])["initialAttributedText"])["text"])
}

func mergeBlocks(grid *GridData, texts []any, tab string) error {
	for _, value := range texts {
		for _, b := range Array(Object(value)["block_datas"]) {
			chunk, err := DecodeGrid(StringValue(Object(b)["related_sheet"]), tab)
			if err != nil {
				return err
			}
			grid.merge(chunk)
		}
	}
	return nil
}

func ReadPublicTasks(ctx context.Context, link, owner string, notify ProgressFunc, client *http.Client, includeCompleted bool, options ...ReadOptions) (model.Document, error) {
	opts := readOptions(options)
	if opts.Conditions != nil {
		if err := ValidatePullConditions(*opts.Conditions); err != nil {
			return model.Document{}, err
		}
	}
	link, err := storage.ValidateURL(link)
	if err != nil {
		return model.Document{}, err
	}
	notify("正在直接读取腾讯文档…")
	session, err := newPublicSession(ctx, link, client)
	if err != nil {
		return model.Document{}, err
	}
	endRow := 1999
	if opts.SchemaOnly {
		endRow = 99
	}
	initial, err := session.load(ctx, map[string]int{"startrow": 0, "endrow": endRow})
	if err != nil {
		return model.Document{}, err
	}
	parsed, _ := url.Parse(link)
	tabID := parsed.Query().Get("tab")
	var tabs []map[string]any
	for _, block := range Array(Object(initial["collab_client_vars"])["header"]) {
		for _, tab := range Array(Object(block)["d"]) {
			tabs = append(tabs, Object(tab))
		}
	}
	if tabID == "" && len(tabs) > 0 {
		tabID = StringValue(tabs[0]["id"])
	}
	var tab map[string]any
	for _, candidate := range tabs {
		if StringValue(candidate["id"]) == tabID {
			tab = candidate
			break
		}
	}
	if tab == nil {
		return model.Document{}, errors.New("链接指定的工作表不存在或没有查看权限。")
	}
	texts := clientTexts(initial)
	maxRow, maxCol := 0, 0
	for _, text := range texts {
		t := Object(text)
		maxRow = max(maxRow, int(Number(t["max_row"])))
		maxCol = max(maxCol, int(Number(t["max_col"])))
	}
	if maxRow < 0 || maxCol < 0 || int64(maxRow)*int64(maxCol) > 2_000_000 || maxRow > 2_000_000 || maxCol > 2_000_000 {
		return model.Document{}, errors.New("工作表范围过大，暂不支持一次读取超过 200 万个单元格。")
	}
	smart := StringValue(tab["type"]) == "smartsheet"
	var grid *GridData
	if smart {
		state, records, err := smartParts(texts, tabID)
		if err != nil {
			return model.Document{}, err
		}
		if state == nil {
			return model.Document{}, errors.New("未取得智能表格的字段信息。")
		}
		for start := 2000; start < maxRow && !opts.SchemaOnly; start += 2000 {
			notify(fmt.Sprintf("正在读取智能表格第 %d 行起的数据…", start+1))
			chunk, err := session.load(ctx, map[string]int{"startrow": start, "endrow": min(start+1999, maxRow-1)})
			if err != nil {
				return model.Document{}, err
			}
			_, extra, err := smartParts(clientTexts(chunk), tabID)
			if err != nil {
				return model.Document{}, err
			}
			for key, value := range extra {
				records[key] = value
			}
		}
		grid, err = DecodeSmart(state, records, parsed.Query().Get("viewId"), opts)
		if err != nil {
			return model.Document{}, err
		}
	} else {
		grid = NewGrid()
		if err := mergeBlocks(grid, texts, tabID); err != nil {
			return model.Document{}, err
		}
		if len(grid.Ranges) == 0 {
			return model.Document{}, errors.New("未取得普通表格的数据块。")
		}
		for row := 0; row < maxRow && !opts.SchemaOnly; row += 256 {
			for col := 0; col < maxCol; col += 32 {
				lastRow, lastCol := min(row+255, maxRow-1), min(col+31, maxCol-1)
				covered := false
				for _, bounds := range grid.Ranges {
					if bounds.FirstRow <= row && bounds.LastRow >= lastRow && bounds.FirstCol <= col && bounds.LastCol >= lastCol {
						covered = true
						break
					}
				}
				if covered {
					continue
				}
				chunk, err := session.load(ctx, map[string]int{"startrow": row, "endrow": lastRow, "block_start_row": row, "block_end_row": lastRow, "block_start_col": col, "block_end_col": lastCol})
				if err != nil {
					return model.Document{}, err
				}
				if err := mergeBlocks(grid, clientTexts(chunk), tabID); err != nil {
					return model.Document{}, err
				}
			}
		}
	}
	notify("正在按拉取条件整理任务…")
	title := StringValue(initial["title"])
	if title == "" {
		title = "腾讯文档"
	}
	return DocumentFromGrid(ctx, grid, title, StringValue(tab["name"]), link, owner, notify, smart, session.client, includeCompleted, opts)
}

func downloadPicture(ctx context.Context, client *http.Client, pic RemoteImage) (model.Picture, error) {
	parsed, err := url.Parse(pic.URL)
	if err != nil || parsed.Scheme != "https" || !strings.HasSuffix(parsed.Hostname(), ".docs.qq.com") || parsed.User != nil {
		return model.Picture{}, errors.New("图片地址不是腾讯文档的 HTTPS 图片地址。")
	}
	var raw []byte
	for attempt := 0; attempt < 2; attempt++ {
		raw, err = fetch(ctx, client, pic.URL, "https://docs.qq.com/", 30*1024*1024)
		if err == nil {
			return PngPicture(raw, pic.Row+1, pic.Col+1, pic.Width, pic.Height)
		}
		if ctx.Err() != nil {
			return model.Picture{}, ctx.Err()
		}
	}
	return model.Picture{}, err
}

func DocumentFromGrid(ctx context.Context, grid *GridData, title, sheetName, link, owner string, notify ProgressFunc, smart bool, client *http.Client, includeCompleted bool, options ...ReadOptions) (model.Document, error) {
	opts := readOptions(options)
	headerRow, ownerCol := -1, -1
	if owner == "" || opts.SchemaOnly {
		headerRow = discoverHeader(grid, smart)
	} else {
		owners := map[int][]int{}
		for position, value := range grid.Cells {
			if position.Row < 100 && OwnerHeader(value) {
				owners[position.Row] = append(owners[position.Row], position.Col)
			}
		}
		for row := 0; row < 100; row++ {
			columns := owners[row]
			if len(columns) > 1 {
				return model.Document{}, errors.New("该表头有多列负责人，请先明确需要筛选的负责人列。")
			}
			if len(columns) == 1 {
				headerRow, ownerCol = row, columns[0]
				break
			}
		}
	}

	if headerRow < 0 {
		return model.Document{}, errors.New("无法识别表头，请使用一行表头、一行一条任务的表格。")
	}
	width := 0
	rowSet := map[int]bool{}
	scannedRows := map[int]bool{}
	for pos, value := range grid.Cells {
		if value != nil {
			width = max(width, pos.Col+1)
			if pos.Row > headerRow {
				scannedRows[pos.Row] = true
			}
		}
		if pos.Row > headerRow {
			rowSet[pos.Row] = true
		}
	}
	for pos := range grid.Images {
		width = max(width, pos.Col+1)
	}
	headers := make([]any, width)
	for col := range headers {
		headers[col] = grid.Cells[CellPosition{headerRow, col}]
	}
	filters, err := compileFilters(headers, sheetName, opts.Conditions)
	if err != nil {
		return model.Document{}, err
	}
	if err = validateFilterSheets([]model.TaskSheet{{Name: sheetName}}, opts.Conditions); err != nil {
		return model.Document{}, err
	}
	numbers := []int{}
	if !opts.SchemaOnly {
		for row := range rowSet {
			numbers = append(numbers, row)
		}
	}
	sort.Ints(numbers)
	rows := []model.TaskRow{}
	positions := map[int]int{}
	for _, row := range numbers {
		if owner != "" && !OwnerMatches(grid.Cells[CellPosition{row, ownerCol}], owner) {
			continue
		}
		values := make([]any, width)
		for col := range values {
			values[col] = DateText(grid.Cells[CellPosition{row, col}], headers[col])
		}
		if !matchesFilters(values, filters) || (!includeCompleted && CompletedTask(headers, values)) {
			continue
		}
		number := row + 1
		if smart {
			number = row
		}
		positions[row] = len(rows)
		rows = append(rows, model.TaskRow{RecordID: grid.RowIDs[row], Number: number, Values: values, Pictures: []model.Picture{}})
	}
	var images []RemoteImage
	for position, pictures := range grid.Images {
		if _, ok := positions[position.Row]; ok {
			images = append(images, pictures...)
		}
	}
	sort.Slice(images, func(i, j int) bool {
		if images[i].Row != images[j].Row {
			return images[i].Row < images[j].Row
		}
		return images[i].Col < images[j].Col
	})
	if len(images) > 0 {
		notify(fmt.Sprintf("已找到 %d 条任务，正在保存 %d 张图片…", len(rows), len(images)))
		if client == nil {
			client = NewHTTPClient()
		}
		var mu sync.Mutex
		var wg sync.WaitGroup
		jobs := make(chan RemoteImage)
		completed := 0
		failures := []string{}
		for worker := 0; worker < min(2, len(images)); worker++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for pic := range jobs {
					picture, err := downloadPicture(ctx, client, pic)
					mu.Lock()
					if err != nil {
						failures = append(failures, fmt.Sprintf("第 %d 行第 %d 列", pic.Row+1, pic.Col+1))
					} else {
						index := positions[pic.Row]
						rows[index].Pictures = append(rows[index].Pictures, picture)
					}
					completed++
					notify(fmt.Sprintf("正在保存图片 %d/%d…", completed, len(images)))
					mu.Unlock()
				}
			}()
		}
		for _, pic := range images {
			select {
			case jobs <- pic:
			case <-ctx.Done():
				close(jobs)
				wg.Wait()
				return model.Document{}, ctx.Err()
			}
		}
		close(jobs)
		wg.Wait()
		if ctx.Err() != nil {
			return model.Document{}, ctx.Err()
		}
		if len(failures) > 0 {
			sort.Strings(failures)
			return model.Document{}, fmt.Errorf("%d 张图片下载失败（%s）。没有覆盖本地旧结果，请重新同步。", len(failures), strings.Join(failures, "、"))
		}
	}
	for i := range rows {
		sort.SliceStable(rows[i].Pictures, func(a, b int) bool { return rows[i].Pictures[a].Column < rows[i].Pictures[b].Column })
	}
	return model.Document{SourceURL: link, Owner: strings.TrimSpace(owner), Title: title, IncludeCompleted: includeCompleted, Warnings: UniqueStrings(grid.Warnings), Sheets: []model.TaskSheet{{StatusOptions: UniqueStrings(grid.StatusOptions), Name: sheetName, HeaderRow: headerRow + 1, Headers: headers, Rows: rows, Scanned: len(scannedRows)}}}, nil
}
