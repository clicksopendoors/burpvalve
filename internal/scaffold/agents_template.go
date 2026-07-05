package scaffold

import (
	"bytes"
	"strings"
	"text/template"
)

type agentsTemplateData struct {
	Beads                   bool
	NTM                     bool
	Orchestrator            bool
	ClaudeOrchestratorRoute bool
	Verifier                bool
	Backpressure            bool
	Docs                    bool
	Plans                   bool
	Log                     bool
	DocsPlansLogs           bool
	StartupRecordLine       string
	AtomicPlanLine          string
	AtomicSplitLine         string
	DocsPlansLogBullets     []string
	PromoteDurableLine      string
	UncertaintyRecordLine   string
}

type orchestratorTemplateData struct {
	Dogfood bool
}

func scaffoldSkips(opts ApplyOptions) map[ScaffoldTarget]bool {
	return map[ScaffoldTarget]bool{
		TargetAgents:       opts.SkipAgents,
		TargetClaude:       opts.SkipClaude,
		TargetOrchestrator: false,
		TargetDocs:         opts.SkipDocs,
		TargetPlans:        opts.SkipPlans,
		TargetLog:          opts.SkipLog,
		TargetBackpressure: opts.SkipBackpressure,
		TargetAttestations: opts.SkipAttestations,
		TargetBeads:        opts.SkipBeads,
		TargetNTM:          opts.SkipNTM,
		TargetPreCommit:    opts.SkipPreCommit,
		TargetHooksPath:    opts.SkipHooksPath,
		TargetTool:         opts.SkipTool,
		TargetToolDocs:     opts.SkipToolDocs,
	}
}

func renderAgentsTemplate(opts ApplyOptions) ([]byte, error) {
	templateBody, err := embeddedTemplates.ReadFile("templates/AGENTS.md.tmpl")
	if err != nil {
		return nil, err
	}
	targets := activeScaffoldTargets(opts.Targets, scaffoldSkips(opts))
	return renderAgentsTemplateBody(string(templateBody), agentsTemplateDataForOptions(targets, opts))
}

func renderAgentsTemplateBody(body string, data agentsTemplateData) ([]byte, error) {
	tmpl, err := template.New("AGENTS.md").Parse(body)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		return nil, err
	}
	return []byte(strings.TrimRight(out.String(), "\n") + "\n"), nil
}

func renderOrchestratorTemplate(opts ApplyOptions) ([]byte, error) {
	templateBody, err := embeddedTemplates.ReadFile("templates/ORCHESTRATOR.md.tmpl")
	if err != nil {
		return nil, err
	}
	return renderOrchestratorTemplateBody(string(templateBody), orchestratorTemplateData{
		Dogfood: opts.Dogfood,
	})
}

func renderOrchestratorTemplateBody(body string, data orchestratorTemplateData) ([]byte, error) {
	tmpl, err := template.New("ORCHESTRATOR.md").Parse(body)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		return nil, err
	}
	return []byte(strings.TrimRight(out.String(), "\n") + "\n"), nil
}

func agentsTemplateDataForOptions(targets scaffoldTargetSet, opts ApplyOptions) agentsTemplateData {
	data := agentsTemplateData{
		Beads:                   targets.has(TargetBeads),
		NTM:                     targets.has(TargetNTM),
		Orchestrator:            targets.has(TargetOrchestrator),
		ClaudeOrchestratorRoute: targets.has(TargetClaude) && effectiveInitClaudeRoute(opts) == ClaudeRouteOrchestratorSkill,
		Verifier:                opts.VerifierConfigured,
		Backpressure:            targets.has(TargetBackpressure),
		Docs:                    targets.has(TargetDocs),
		Plans:                   targets.has(TargetPlans),
		Log:                     targets.has(TargetLog),
	}
	data.DocsPlansLogs = data.Docs || data.Plans || data.Log
	data.StartupRecordLine = startupRecordLine(data)
	data.AtomicPlanLine = atomicPlanLine(data)
	data.AtomicSplitLine = atomicSplitLine(data)
	data.DocsPlansLogBullets = docsPlansLogBullets(data)
	data.PromoteDurableLine = promoteDurableLine(data)
	data.UncertaintyRecordLine = uncertaintyRecordLine(data)
	return data
}

func startupRecordLine(data agentsTemplateData) string {
	switch {
	case data.Docs && data.Plans:
		return "Record durable decisions in `/docs/` or `/plans/`, not chat."
	case data.Docs:
		return "Record durable decisions in `/docs/`, not chat."
	case data.Plans:
		return "Record durable plans and decisions in `/plans/`, not chat."
	case data.Log:
		return "Record dated work notes in `/log/`, not chat."
	default:
		return ""
	}
}

func atomicPlanLine(data agentsTemplateData) string {
	switch {
	case data.Plans && data.Beads:
		return "All plans must include a single commit bead for each atomic feature or bug fix."
	case data.Plans:
		return "All plans must isolate each atomic feature or bug fix into one commit-sized unit."
	case data.Beads:
		return "Use one bead for each atomic feature or bug fix."
	default:
		return ""
	}
}

func atomicSplitLine(data agentsTemplateData) string {
	if data.Backpressure {
		return "If staged changes contain multiple atomic units, split the commit before running the backpressure verifier."
	}
	return "If staged changes contain multiple atomic units, split the commit before committing."
}

func docsPlansLogBullets(data agentsTemplateData) []string {
	var bullets []string
	if data.Docs {
		bullets = append(bullets, "`/docs/` stores durable project knowledge: architecture notes, vocabulary, decisions, runbooks, and research summaries.")
	}
	if data.Plans {
		if data.Beads {
			bullets = append(bullets, "`/plans/` stores strategic and implementation plans. Actionable work from plans belongs in beads.")
		} else {
			bullets = append(bullets, "`/plans/` stores strategic and implementation plans.")
		}
	}
	if data.Log {
		bullets = append(bullets, "`/log/` stores dated work logs, investigation notes, debugging transcripts, and post-run summaries.")
	}
	return bullets
}

func promoteDurableLine(data agentsTemplateData) string {
	if !data.Log {
		return ""
	}
	switch {
	case data.Docs && data.Plans:
		return "Promote durable decisions from `/log/` to `/docs/` or `/plans/`."
	case data.Docs:
		return "Promote durable decisions from `/log/` to `/docs/`."
	case data.Plans:
		return "Promote durable decisions from `/log/` to `/plans/`."
	default:
		return ""
	}
}

func uncertaintyRecordLine(data agentsTemplateData) string {
	targets := []string{}
	if data.Beads {
		targets = append(targets, "the active bead")
	}
	if data.Plans {
		targets = append(targets, "a plan")
	}
	if data.Log {
		targets = append(targets, "a log entry")
	}
	if len(targets) == 0 {
		return ""
	}
	return "Record the uncertainty in " + joinHumanList(targets) + "."
}

func agentsRepairHeadings(opts ApplyOptions) []string {
	targets := activeScaffoldTargets(opts.Targets, scaffoldSkips(opts))
	data := agentsTemplateDataForOptions(targets, opts)
	headings := []string{"Agent Startup"}
	if data.Beads {
		headings = append(headings, "Beads")
	}
	headings = append(headings, "Taking Orders And Handoffs", "Atomic Work And Commits")
	if data.Backpressure {
		headings = append(headings, "Burpvalve Gate Choreography", "Verifier Work")
	}
	if data.NTM {
		headings = append(headings, "NTM Session Naming")
	}
	if data.Verifier {
		headings = append(headings, "Verifier Orchestration")
	}
	if data.Backpressure {
		headings = append(headings, "Backpressure")
	}
	headings = append(headings, "Definition Of Done")
	if data.DocsPlansLogs {
		headings = append(headings, "Docs, Plans, And Logs")
	}
	headings = append(headings, "Uncertainty", "File Coordination")
	return headings
}

func joinHumanList(values []string) string {
	switch len(values) {
	case 0:
		return ""
	case 1:
		return values[0]
	case 2:
		return values[0] + " or " + values[1]
	default:
		return strings.Join(values[:len(values)-1], ", ") + ", or " + values[len(values)-1]
	}
}
