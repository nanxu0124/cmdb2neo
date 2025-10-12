package rca

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strings"
	"text/template"
)

//go:embed prompt.tmpl
var promptTemplateText string

var promptTemplate = template.Must(template.New("rca_prompt").Parse(promptTemplateText))

// PromptOptions 控制提示词渲染行为。
type PromptOptions struct {
	AssistantRole        string
	Language             string
	OutputExpectation    string
	MaxAppOutages        int
	MaxAffectedNodes     int
	MaxCandidates        int
	MaxExplainedEventIDs int
	MaxPaths             int
	MaxImpactsPerLevel   int
	MaxEventsPerImpact   int
}

// DefaultPromptOptions 返回默认提示词配置。
func DefaultPromptOptions() PromptOptions {
	return PromptOptions{
		AssistantRole:        "一名资深 SRE 值班工程师",
		Language:             "zh-CN",
		OutputExpectation:    "请以 JSON 数组形式返回 {\"cause\", \"confidence\", \"coverage\", \"summary\", \"next_action\"} 字段。",
		MaxAppOutages:        3,
		MaxAffectedNodes:     5,
		MaxCandidates:        5,
		MaxExplainedEventIDs: 6,
		MaxPaths:             5,
		MaxImpactsPerLevel:   5,
		MaxEventsPerImpact:   5,
	}
}

// RenderPrompt 根据 Result 及配置渲染出大模型指令。
func RenderPrompt(result Result, opts PromptOptions) string {
	defaults := DefaultPromptOptions()
	if opts.AssistantRole == "" {
		opts.AssistantRole = defaults.AssistantRole
	}
	if opts.Language == "" {
		opts.Language = defaults.Language
	}
	if opts.OutputExpectation == "" {
		opts.OutputExpectation = defaults.OutputExpectation
	}
	if opts.MaxAppOutages == 0 {
		opts.MaxAppOutages = defaults.MaxAppOutages
	}
	if opts.MaxAffectedNodes == 0 {
		opts.MaxAffectedNodes = defaults.MaxAffectedNodes
	}
	if opts.MaxCandidates == 0 {
		opts.MaxCandidates = defaults.MaxCandidates
	}
	if opts.MaxExplainedEventIDs == 0 {
		opts.MaxExplainedEventIDs = defaults.MaxExplainedEventIDs
	}
	if opts.MaxPaths == 0 {
		opts.MaxPaths = defaults.MaxPaths
	}
	if opts.MaxImpactsPerLevel == 0 {
		opts.MaxImpactsPerLevel = defaults.MaxImpactsPerLevel
	}
	if opts.MaxEventsPerImpact == 0 {
		opts.MaxEventsPerImpact = defaults.MaxEventsPerImpact
	}

	trimmed := trimResultForPrompt(result, opts)

	payload, err := json.MarshalIndent(trimmed, "", "  ")
	if err != nil {
		payload = []byte("{}")
	}

	data := promptTemplateData{
		Options:     opts,
		Payload:     trimmed,
		PayloadJSON: string(payload),
	}

	var sb strings.Builder
	if err := promptTemplate.Execute(&sb, data); err != nil {
		return fallbackPrompt(opts, string(payload))
	}
	return sb.String()
}

type promptPayload struct {
	AppOutages []AppOutage `json:"app_outages,omitempty"`
	Candidates []Candidate `json:"candidates"`
	Paths      []AlarmPath `json:"paths,omitempty"`
}

type promptTemplateData struct {
	Options     PromptOptions
	Payload     promptPayload
	PayloadJSON string
}

func trimResultForPrompt(result Result, opts PromptOptions) promptPayload {
	payload := promptPayload{}

	if len(result.AppOutages) > 0 {
		limit := min(len(result.AppOutages), opts.MaxAppOutages)
		payload.AppOutages = make([]AppOutage, 0, limit)
		for i := 0; i < limit; i++ {
			outage := result.AppOutages[i]
			if opts.MaxAffectedNodes > 0 && len(outage.AffectedNodes) > opts.MaxAffectedNodes {
				outage.AffectedNodes = append([]AppOutageNode(nil), outage.AffectedNodes[:opts.MaxAffectedNodes]...)
			} else {
				outage.AffectedNodes = append([]AppOutageNode(nil), outage.AffectedNodes...)
			}
			payload.AppOutages = append(payload.AppOutages, outage)
		}
	}

	if len(result.Candidates) > 0 {
		limit := min(len(result.Candidates), opts.MaxCandidates)
		payload.Candidates = make([]Candidate, 0, limit)
		for i := 0; i < limit; i++ {
			cand := result.Candidates[i]
			if opts.MaxExplainedEventIDs > 0 && len(cand.Explained) > opts.MaxExplainedEventIDs {
				cand.Explained = append([]string(nil), cand.Explained[:opts.MaxExplainedEventIDs]...)
			} else {
				cand.Explained = append([]string(nil), cand.Explained...)
			}
			payload.Candidates = append(payload.Candidates, cand)
		}
	}

	selectedKeys := make(map[string]struct{}, len(payload.Candidates))
	for _, cand := range payload.Candidates {
		selectedKeys[cand.Node.Key] = struct{}{}
	}

	if len(result.Paths) > 0 && len(selectedKeys) > 0 {
		filtered := make([]AlarmPath, 0, len(result.Paths))
		for _, path := range result.Paths {
			if _, ok := selectedKeys[path.Candidate.Key]; !ok {
				continue
			}
			filtered = append(filtered, trimAlarmPath(path, opts))
			if opts.MaxPaths > 0 && len(filtered) >= opts.MaxPaths {
				break
			}
		}
		payload.Paths = filtered
	}

	return payload
}

func trimAlarmPath(path AlarmPath, opts PromptOptions) AlarmPath {
	trimmed := AlarmPath{Candidate: path.Candidate}
	trimmed.Impacts = trimImpacts(path.Impacts, opts)
	return trimmed
}

func trimImpacts(src []PathImpact, opts PromptOptions) []PathImpact {
	if len(src) == 0 {
		return nil
	}
	limit := len(src)
	if opts.MaxImpactsPerLevel > 0 && limit > opts.MaxImpactsPerLevel {
		limit = opts.MaxImpactsPerLevel
	}
	impacts := make([]PathImpact, 0, limit)
	for i := 0; i < limit; i++ {
		s := src[i]
		impact := PathImpact{Node: s.Node}
		if len(s.Events) > 0 {
			limitEvents := len(s.Events)
			if opts.MaxEventsPerImpact > 0 && limitEvents > opts.MaxEventsPerImpact {
				limitEvents = opts.MaxEventsPerImpact
			}
			events := make([]AlarmEventRef, 0, limitEvents)
			for j := 0; j < limitEvents; j++ {
				events = append(events, s.Events[j])
			}
			sort.Slice(events, func(i, j int) bool {
				if events[i].Occurred.Equal(events[j].Occurred) {
					return events[i].ID < events[j].ID
				}
				return events[i].Occurred.Before(events[j].Occurred)
			})
			impact.Events = events
		}
		impact.Impacts = trimImpacts(s.Impacts, opts)
		impacts = append(impacts, impact)
	}
	return impacts
}

func fallbackPrompt(opts PromptOptions, payload string) string {
	var sb strings.Builder
	sb.WriteString("Role: ")
	sb.WriteString(opts.AssistantRole)
	sb.WriteString("\nStructured Data:\n")
	sb.WriteString(payload)
	sb.WriteString("\n")
	return sb.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
