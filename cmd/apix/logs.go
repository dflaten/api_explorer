package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
	"golang.org/x/term"
)

const redactedValue = "<redacted>"

const (
	ansiReset   = "\033[0m"
	ansiCyan    = "\033[36m"
	ansiGreen   = "\033[32m"
	ansiYellow  = "\033[33m"
	ansiMagenta = "\033[35m"
	ansiDim     = "\033[2m"
	ansiBoldRed = "\033[1;31m"

	boxTopLeft     = "┌"
	boxTopRight    = "┐"
	boxBottomLeft  = "└"
	boxBottomRight = "┘"
	boxHorizontal  = "─"
	boxVertical    = "│"
)

var (
	unsafeLogNamePattern = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
	numberScalarPattern  = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)
	timestampPattern     = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T`)
	sensitiveFieldNames  = map[string]bool{
		"access_token":  true,
		"api_key":       true,
		"apikey":        true,
		"authorization": true,
		"client_secret": true,
		"password":      true,
		"refresh_token": true,
		"secret":        true,
		"token":         true,
	}
)

type requestLogEntry struct {
	Name         string              `json:"name"`
	API          string              `json:"api"`
	Endpoint     string              `json:"endpoint"`
	Method       string              `json:"method"`
	URL          string              `json:"url"`
	Path         string              `json:"path"`
	Params       map[string]any      `json:"params,omitempty"`
	PathParams   map[string]any      `json:"path_params,omitempty"`
	QueryParams  map[string]any      `json:"query_params,omitempty"`
	Headers      map[string]string   `json:"headers,omitempty"`
	Body         any                 `json:"body,omitempty"`
	StatusCode   *int                `json:"status_code,omitempty"`
	Success      bool                `json:"success"`
	Error        string              `json:"error,omitempty"`
	StartedAt    string              `json:"started_at"`
	CompletedAt  string              `json:"completed_at"`
	DurationMS   int64               `json:"duration_ms"`
	Operations   []requestLogEvent   `json:"operations"`
	ResponseInfo *requestLogResponse `json:"response,omitempty"`
}

type requestLogEvent struct {
	Name      string `json:"name"`
	Timestamp string `json:"timestamp"`
}

type requestLogResponse struct {
	Headers map[string]string `json:"headers,omitempty"`
}

type requestLogSummary struct {
	Path       string
	Endpoint   string
	Method     string
	StatusCode *int
	StartedAt  string
	sortTime   time.Time
	DurationMS int64
}

type requestLogDisplay struct {
	Name       string            `yaml:"name"`
	API        string            `yaml:"api"`
	Endpoint   string            `yaml:"endpoint"`
	Method     string            `yaml:"method"`
	StatusCode *int              `yaml:"status_code,omitempty"`
	Success    bool              `yaml:"success"`
	Error      string            `yaml:"error,omitempty"`
	DurationMS int64             `yaml:"duration_ms"`
	Request    requestLogRequest `yaml:"request"`
	Timing     requestLogTiming  `yaml:"timing"`
	Operations []requestLogEvent `yaml:"operations,omitempty"`
	Response   any               `yaml:"response,omitempty"`
}

type requestLogRequest struct {
	URL         string            `yaml:"url"`
	Path        string            `yaml:"path"`
	PathParams  map[string]any    `yaml:"path_params,omitempty"`
	QueryParams map[string]any    `yaml:"query_params,omitempty"`
	Headers     map[string]string `yaml:"headers,omitempty"`
	Body        any               `yaml:"body,omitempty"`
}

type requestLogTiming struct {
	StartedAt   string `yaml:"started_at"`
	CompletedAt string `yaml:"completed_at"`
}

func defaultLogDir() string { return filepath.Join(defaultConfigHome(), "logs") }

func configLogName(configPath string) string {
	base := strings.TrimSuffix(filepath.Base(configPath), filepath.Ext(configPath))
	if base == "" {
		return "config"
	}
	return sanitizeLogName(base)
}

func logRequest(configPath string, definition *RequestDefinition, response *http.Response, requestErr error, startedAt, completedAt time.Time) error {
	apiName := configLogName(configPath)
	statusCode := (*int)(nil)
	success := false
	responseInfo := (*requestLogResponse)(nil)
	if response != nil {
		status := response.StatusCode
		statusCode = &status
		success = status >= 200 && status < 400
		responseInfo = &requestLogResponse{Headers: redactHeaders(responseHeaders(response.Header))}
	}
	errorText := ""
	if requestErr != nil {
		errorText = requestErr.Error()
	}

	name := requestLogName(definition, completedAt)
	entry := requestLogEntry{
		Name:         name,
		API:          apiName,
		Endpoint:     definition.Endpoint,
		Method:       definition.Method,
		URL:          redactURL(definition.FullURL),
		Path:         definition.Path,
		PathParams:   redactMap(definition.PathParams.Map()),
		QueryParams:  redactMap(definition.QueryParams.Map()),
		Headers:      redactHeaders(definition.EffectiveHeaders),
		Body:         redactValue(definition.Body),
		StatusCode:   statusCode,
		Success:      success,
		Error:        errorText,
		StartedAt:    startedAt.Format(time.RFC3339Nano),
		CompletedAt:  completedAt.Format(time.RFC3339Nano),
		DurationMS:   completedAt.Sub(startedAt).Milliseconds(),
		Operations:   []requestLogEvent{{Name: "request_started", Timestamp: startedAt.Format(time.RFC3339Nano)}, {Name: "request_completed", Timestamp: completedAt.Format(time.RFC3339Nano)}},
		ResponseInfo: responseInfo,
	}

	directory := filepath.Join(defaultLogDir(), apiName)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(entry); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, name+".json"), output.Bytes(), 0o600)
}

func requestLogName(definition *RequestDefinition, timestamp time.Time) string {
	parts := []string{timestamp.Format("20060102T150405.000000000"), strings.ToLower(definition.Method), definition.Endpoint}
	if definition.Path != "" && definition.Path != "/" {
		parts = append(parts, strings.Trim(definition.Path, "/"))
	}
	return sanitizeLogName(strings.Join(parts, "_"))
}

func sanitizeLogName(value string) string {
	result := unsafeLogNamePattern.ReplaceAllString(value, "_")
	result = strings.Trim(result, "._-")
	if result == "" {
		return "request"
	}
	if len(result) > 180 {
		return result[:180]
	}
	return result
}

func requestLogPath(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	if parsed.RawQuery == "" {
		return parsed.EscapedPath()
	}
	return parsed.EscapedPath() + "?" + parsed.RawQuery
}

func redactURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	if parsed.User != nil {
		parsed.User = url.User(redactedValue)
	}
	query := parsed.Query()
	for key := range query {
		if isSensitiveName(key) {
			query.Set(key, redactedValue)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func redactMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]any, len(values))
	for key, value := range values {
		if isSensitiveName(key) {
			result[key] = redactedValue
		} else {
			result[key] = redactValue(value)
		}
	}
	return result
}

func redactValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return redactMap(typed)
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = redactValue(item)
		}
		return result
	default:
		return value
	}
}

func isSensitiveName(name string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(name, "-", "_"))
	return sensitiveFieldNames[normalized] || strings.Contains(normalized, "token") || strings.Contains(normalized, "secret") || strings.Contains(normalized, "password")
}

func executeWithLog(client *APIClient, definition *RequestDefinition) (*http.Response, error) {
	startedAt := time.Now()
	response, err := client.execute(definition)
	completedAt := time.Now()
	if logErr := logRequest(client.ConfigPath, definition, response, err, startedAt, completedAt); logErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to write request log: %s\n", logErr)
	}
	return response, err
}

func listRequestLogs(configPath string) ([]requestLogSummary, error) {
	directory := filepath.Join(defaultLogDir(), configLogName(configPath))
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	logs := []requestLogSummary{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		summary, err := readRequestLogSummary(path)
		if err != nil {
			continue
		}
		summary.Path = path
		if summary.sortTime.IsZero() {
			info, statErr := entry.Info()
			if statErr == nil {
				summary.sortTime = info.ModTime()
			}
		}
		logs = append(logs, summary)
	}
	sort.Slice(logs, func(left, right int) bool {
		return logs[left].sortTime.After(logs[right].sortTime)
	})
	return logs, nil
}

func readRequestLogSummary(path string) (requestLogSummary, error) {
	entry, err := readRequestLogEntry(path)
	if err != nil {
		return requestLogSummary{}, err
	}
	return requestLogSummary{
		Endpoint:   entry.Endpoint,
		Method:     entry.Method,
		StatusCode: entry.StatusCode,
		StartedAt:  entry.StartedAt,
		sortTime:   requestLogSortTime(entry.StartedAt),
		DurationMS: entry.DurationMS,
	}, nil
}

func requestLogSortTime(value string) time.Time {
	timestamp, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return timestamp
}

func readRequestLogEntry(path string) (requestLogEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return requestLogEntry{}, err
	}
	var entry requestLogEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return requestLogEntry{}, err
	}
	return entry, nil
}

func printRequestLogs(configPath string) error {
	logs, err := listRequestLogs(configPath)
	if err != nil {
		return err
	}
	if len(logs) == 0 {
		fmt.Printf("No request logs found for %s.\n", configLogName(configPath))
		return nil
	}
	if !stdinIsTerminal() {
		printRequestLogList(configLogName(configPath), logs)
		return nil
	}
	return browseRequestLogs(configLogName(configPath), logs)
}

func requestLogEntryYAML(entry requestLogEntry) (string, error) {
	queryParams := entry.QueryParams
	if len(queryParams) == 0 {
		queryParams = entry.Params
	}
	output, err := yaml.Marshal(requestLogDisplay{
		Name:       entry.Name,
		API:        entry.API,
		Endpoint:   entry.Endpoint,
		Method:     entry.Method,
		StatusCode: entry.StatusCode,
		Success:    entry.Success,
		Error:      entry.Error,
		DurationMS: entry.DurationMS,
		Request: requestLogRequest{
			URL:         entry.URL,
			Path:        entry.Path,
			PathParams:  entry.PathParams,
			QueryParams: queryParams,
			Headers:     entry.Headers,
			Body:        entry.Body,
		},
		Timing: requestLogTiming{
			StartedAt:   entry.StartedAt,
			CompletedAt: entry.CompletedAt,
		},
		Operations: entry.Operations,
		Response:   entry.ResponseInfo,
	})
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func shouldColorTerminalOutput() bool {
	switch terminalColorMode {
	case "always":
		return true
	case "never":
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("CLICOLOR") == "0" {
		return false
	}
	if force := os.Getenv("CLICOLOR_FORCE"); force != "" && force != "0" {
		return true
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return stdoutIsTerminal()
}

func colorizeYAML(input string) string {
	lines := strings.SplitAfter(input, "\n")
	var output strings.Builder
	for _, line := range lines {
		output.WriteString(colorizeYAMLLine(line))
	}
	return output.String()
}

func colorizeYAMLLine(line string) string {
	content, newline := strings.TrimSuffix(line, "\n"), ""
	if content != line {
		newline = "\n"
	}
	if strings.TrimSpace(content) == "" {
		return line
	}
	indentLength := len(content) - len(strings.TrimLeft(content, " "))
	indent, rest := content[:indentLength], content[indentLength:]
	prefix := ""
	if strings.HasPrefix(rest, "- ") {
		prefix = ansiDim + "- " + ansiReset
		rest = strings.TrimPrefix(rest, "- ")
	}
	key, value, found := strings.Cut(rest, ":")
	if !found {
		return indent + prefix + colorizeYAMLScalar(rest) + newline
	}
	result := indent + prefix + ansiCyan + key + ansiReset + ":"
	if value == "" {
		return result + newline
	}
	if strings.HasPrefix(value, " ") {
		return result + " " + colorizeYAMLScalar(strings.TrimPrefix(value, " ")) + newline
	}
	return result + colorizeYAMLScalar(value) + newline
}

func colorizeYAMLScalar(value string) string {
	trimmed := strings.TrimSpace(value)
	switch {
	case trimmed == redactedValue:
		return ansiBoldRed + value + ansiReset
	case trimmed == "true" || trimmed == "false":
		return ansiMagenta + value + ansiReset
	case trimmed == "null":
		return ansiDim + value + ansiReset
	case numberScalarPattern.MatchString(trimmed):
		return ansiYellow + value + ansiReset
	case timestampPattern.MatchString(trimmed):
		return ansiDim + value + ansiReset
	case strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://"):
		return ansiGreen + value + ansiReset
	default:
		return ansiGreen + value + ansiReset
	}
}

func printRequestLogList(apiName string, logs []requestLogSummary) {
	fmt.Printf("Request logs for %s:\n", apiName)
	for index, entry := range logs {
		fmt.Printf("  %2d. %s\n", index+1, requestLogSummaryLine(entry))
	}
}

func browseRequestLogs(apiName string, logs []requestLogSummary) error {
	fd := int(os.Stdin.Fd())
	previousState, err := term.MakeRaw(fd)
	if err != nil {
		return err
	}
	defer term.Restore(fd, previousState)
	defer fmt.Print("\033[?25h")

	selected, offset := 0, 0
	for {
		index, nextSelected, nextOffset, err := selectRequestLog(apiName, logs, selected, offset)
		if err != nil {
			return err
		}
		selected, offset = nextSelected, nextOffset
		if index < 0 {
			fmt.Print("\033[2J\033[H")
			return nil
		}
		entry, err := readRequestLogEntry(logs[index].Path)
		if err != nil {
			return err
		}
		text, err := requestLogEntryYAML(entry)
		if err != nil {
			return err
		}
		if err := viewRequestLog(apiName, logs[index], text, shouldColorTerminalOutput()); err != nil {
			return err
		}
	}
}

func selectRequestLog(apiName string, logs []requestLogSummary, selected, offset int) (int, int, int, error) {
	fd := int(os.Stdin.Fd())
	for {
		width, height, err := term.GetSize(fd)
		if err != nil || width <= 0 || height <= 0 {
			width, height = 100, 24
		}
		visible := height - 5
		if visible < 1 {
			visible = 1
		}
		if selected < offset {
			offset = selected
		}
		if selected >= offset+visible {
			offset = selected - visible + 1
		}
		renderRequestLogSelector(apiName, logs, selected, offset, visible, width)

		key, err := readTerminalKey()
		if err != nil {
			return -1, selected, offset, err
		}
		switch key {
		case "up":
			if selected > 0 {
				selected--
			}
		case "down":
			if selected < len(logs)-1 {
				selected++
			}
		case "enter":
			return selected, selected, offset, nil
		case "quit":
			return -1, selected, offset, nil
		}
	}
}

func readTerminalKey() (string, error) {
	buffer := []byte{0}
	if _, err := os.Stdin.Read(buffer); err != nil {
		return "", err
	}
	switch buffer[0] {
	case '\r', '\n':
		return "enter", nil
	case 'q', 'Q', 3:
		return "quit", nil
	case 'k', 'K':
		return "up", nil
	case 'j', 'J':
		return "down", nil
	case 0x1b:
		sequence := []byte{0, 0}
		if _, err := os.Stdin.Read(sequence); err != nil {
			return "", err
		}
		if sequence[0] == '[' {
			if sequence[1] == 'A' {
				return "up", nil
			}
			if sequence[1] == 'B' {
				return "down", nil
			}
			if sequence[1] == 'H' {
				return "home", nil
			}
			if sequence[1] == 'F' {
				return "end", nil
			}
			if sequence[1] >= '1' && sequence[1] <= '8' {
				terminator := []byte{0}
				if _, err := os.Stdin.Read(terminator); err != nil {
					return "", err
				}
				if terminator[0] == '~' {
					switch sequence[1] {
					case '1', '7':
						return "home", nil
					case '4', '8':
						return "end", nil
					case '5':
						return "page_up", nil
					case '6':
						return "page_down", nil
					}
				}
			}
		}
		return "quit", nil
	}
	return "", nil
}

func viewRequestLog(apiName string, summary requestLogSummary, text string, color bool) error {
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	offset := 0
	for {
		width, height, err := term.GetSize(int(os.Stdin.Fd()))
		if err != nil || width <= 0 || height <= 0 {
			width, height = 100, 24
		}
		bodyHeight := height - 5
		if bodyHeight < 1 {
			bodyHeight = 1
		}
		maxOffset := len(lines) - bodyHeight
		if maxOffset < 0 {
			maxOffset = 0
		}
		if offset > maxOffset {
			offset = maxOffset
		}
		renderRequestLogViewer(apiName, summary, lines, offset, bodyHeight, width, color)
		key, err := readTerminalKey()
		if err != nil {
			return err
		}
		switch key {
		case "up":
			if offset > 0 {
				offset--
			}
		case "down":
			if offset < maxOffset {
				offset++
			}
		case "page_up":
			offset -= bodyHeight
			if offset < 0 {
				offset = 0
			}
		case "page_down":
			offset += bodyHeight
			if offset > maxOffset {
				offset = maxOffset
			}
		case "home":
			offset = 0
		case "end":
			offset = maxOffset
		case "quit":
			return nil
		}
	}
}

func renderRequestLogViewer(apiName string, summary requestLogSummary, lines []string, offset, bodyHeight, width int, color bool) {
	if width < 20 {
		width = 20
	}
	innerWidth := width - 4
	if innerWidth < 1 {
		innerWidth = 1
	}
	end := offset + bodyHeight
	if end > len(lines) {
		end = len(lines)
	}
	title := fmt.Sprintf("%s / %s", apiName, summary.Endpoint)
	footer := "Up/Down scroll  PgUp/PgDn page  q back to logs"
	if len(lines) > bodyHeight {
		footer = fmt.Sprintf("%s  lines %d-%d/%d", footer, offset+1, end, len(lines))
	}

	fmt.Print("\033[?25l\033[2J\033[H")
	fmt.Printf("%s%s%s\r\n", boxTopLeft, strings.Repeat(boxHorizontal, width-2), boxTopRight)
	fmt.Printf("%s %s %s\r\n", boxVertical, padTerminalLine(truncateTerminalLine(title, innerWidth), innerWidth), boxVertical)
	fmt.Printf("%s%s%s\r\n", boxVertical, strings.Repeat(boxHorizontal, width-2), boxVertical)
	for index := offset; index < end; index++ {
		line := truncateTerminalLine(lines[index], innerWidth)
		line = padTerminalLine(line, innerWidth)
		if color {
			line = colorizeYAMLLine(line)
		}
		fmt.Printf("%s %s %s\r\n", boxVertical, line, boxVertical)
	}
	for index := end; index < offset+bodyHeight; index++ {
		fmt.Printf("%s %s %s\r\n", boxVertical, strings.Repeat(" ", innerWidth), boxVertical)
	}
	fmt.Printf("%s%s%s\r\n", boxBottomLeft, strings.Repeat(boxHorizontal, width-2), boxBottomRight)
	fmt.Printf("%s\r\n", truncateTerminalLine(footer, width))
}

func renderRequestLogSelector(apiName string, logs []requestLogSummary, selected, offset, visible, width int) {
	fmt.Print("\033[?25l\033[2J\033[H")
	fmt.Printf("Request logs for %s\r\n", apiName)
	fmt.Print("Use Up/Down to choose a log, Enter to view, q to exit.\r\n")
	fmt.Print("\r\n")
	end := offset + visible
	if end > len(logs) {
		end = len(logs)
	}
	for index := offset; index < end; index++ {
		prefix := "  "
		if index == selected {
			prefix = "> "
			fmt.Print("\033[7m")
		}
		fmt.Print(truncateTerminalLine(prefix+requestLogSummaryLine(logs[index]), width))
		if index == selected {
			fmt.Print("\033[0m")
		}
		fmt.Print("\r\n")
	}
	if len(logs) > visible {
		fmt.Printf("\r\nShowing %d-%d of %d\r\n", offset+1, end, len(logs))
	}
}

func requestLogSummaryLine(entry requestLogSummary) string {
	status := "error"
	if entry.StatusCode != nil {
		status = strconv.Itoa(*entry.StatusCode)
	}
	return fmt.Sprintf("%-30s %-6s %-4s %6dms %s", entry.Endpoint, entry.Method, status, entry.DurationMS, entry.StartedAt)
}

func truncateTerminalLine(value string, width int) string {
	if width <= 0 || len([]rune(value)) <= width {
		return value
	}
	if width == 1 {
		return string([]rune(value)[:1])
	}
	runes := []rune(value)
	return string(runes[:width-1]) + "."
}

func padTerminalLine(value string, width int) string {
	length := len([]rune(value))
	if width <= 0 || length >= width {
		return value
	}
	return value + strings.Repeat(" ", width-length)
}

func stdinIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func stdoutIsTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}
