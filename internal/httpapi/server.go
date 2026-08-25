package httpapi

import (
	"coldchain/internal/application"
	"coldchain/internal/domain"
	"coldchain/internal/risk"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed static/index.html static/app.js static/style.css
var assets embed.FS

type Server struct {
	app *application.Service
	mux *http.ServeMux
}

func New(app *application.Service) *Server {
	s := &Server{app: app, mux: http.NewServeMux()}
	s.routes()
	return s
}
func (s *Server) Handler() http.Handler { return s.mux }
func (s *Server) routes() {
	s.mux.HandleFunc("/", s.page)
	s.mux.Handle("/static/", http.FileServer(http.FS(assets)))
	s.mux.HandleFunc("/healthz", s.healthz)
	s.mux.HandleFunc("/api/cases", s.cases)
	s.mux.HandleFunc("/api/cases/", s.caseAction)
	s.mux.HandleFunc("/api/credentials/", s.verify)
	s.mux.HandleFunc("/api/credentials", s.verify)
	s.mux.HandleFunc("/api/credential-verifications", s.verificationRecords)
}
func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	write(w, 200, map[string]string{"status": "ok"})
}
func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	b, _ := assets.ReadFile("static/index.html")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(b)
}
func decode(r *http.Request, v any) error {
	if r.ContentLength > 1<<20 {
		return errors.New("请求过大")
	}
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		return errors.New("Content-Type 必须为 application/json")
	}
	d := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	d.DisallowUnknownFields()
	return d.Decode(v)
}
func write(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
func status(err error) int {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return 404
	case errors.Is(err, domain.ErrConflict):
		return 409
	case errors.Is(err, domain.ErrInvalid), errors.Is(err, domain.ErrInvalidState), errors.Is(err, domain.ErrRiskUnresolved), errors.Is(err, domain.ErrImmutable):
		return 422
	default:
		return 400
	}
}
func errorBody(err error) map[string]any {
	out := map[string]any{"error": err.Error()}
	var ce *domain.CaseConflictError
	if errors.As(err, &ce) {
		out["code"] = ce.Code
		out["conflict"] = ce
		out["conflictingCaseNumber"] = ce.ConflictingCase
		out["windowStart"] = ce.WindowStart
		out["windowEnd"] = ce.WindowEnd
		return out
	}
	var de *domain.EvidenceDuplicateError
	if errors.As(err, &de) {
		out["code"] = "DUPLICATE_EVIDENCE"
		out["revisionNumber"] = de.RevisionNumber
		out["evidenceRef"] = de.EvidenceRef
	}
	var calibration *domain.CalibrationCoverageError
	if errors.As(err, &calibration) {
		out["code"] = "CALIBRATION_NOT_COVERED"
		out["probeId"] = calibration.ProbeID
		out["certificateRef"] = calibration.CertificateRef
		out["missingBoundaries"] = calibration.MissingBoundary
	}
	var fingerprint *domain.RiskFingerprintConflictError
	if errors.As(err, &fingerprint) {
		out["code"] = "RISK_FINGERPRINT_CONFLICT"
		out["providedFingerprint"] = fingerprint.ProvidedFingerprint
		out["currentFingerprint"] = fingerprint.CurrentFingerprint
		out["currentVersion"] = fingerprint.CurrentVersion
	}
	var precheck *domain.EvidencePrecheckConflictError
	if errors.As(err, &precheck) {
		out["code"] = "EVIDENCE_PRECHECK_CONFLICT"
		out["providedFingerprint"] = precheck.ProvidedFingerprint
		out["currentFingerprint"] = precheck.CurrentFingerprint
		out["currentVersion"] = precheck.CurrentVersion
	}
	var remediation *domain.RemediationGateError
	if errors.As(err, &remediation) {
		out["code"] = "REMEDIATION_GATE_FAILED"
		out["reasons"] = remediation.Reasons
	}
	return out
}
func validatePublicBasics(containers []domain.SampleContainer, probes []domain.TemperatureProbe) error {
	legacyBatch := len(containers) > 1
	for _, c := range containers {
		if strings.TrimSpace(c.SampleCategory) != "" {
			legacyBatch = false
		}
	}
	for i, p := range probes {
		if strings.TrimSpace(p.CertificateRef) == "" && !legacyBatch {
			return fmt.Errorf("%w:探头[%d].certificateRef不能为空", domain.ErrInvalid, i)
		}
	}
	return nil
}
func validatePublicEvidence(rs []domain.TransportEvidenceRevision) error {
	for i, r := range rs {
		if len(r.Readings) < 3 {
			return fmt.Errorf("%w:证据[%d]至少需要起止和一条中间读数", domain.ErrInvalid, i)
		}
		if !r.Readings[0].At.Equal(r.SegmentStart) || !r.Readings[len(r.Readings)-1].At.Equal(r.SegmentEnd) {
			return fmt.Errorf("%w:证据[%d]必须包含分段起止读数", domain.ErrInvalid, i)
		}
	}
	return nil
}

type createReq struct {
	CaseNumber, SenderName, ReceiverName string
	HandoffWindowStart, HandoffWindowEnd time.Time
}

func (s *Server) cases(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		f, e := parseFilter(r.URL.Query())
		if e != nil {
			write(w, status(e), map[string]any{"error": e.Error(), "code": "INVALID_FILTER"})
			return
		}
		out, e := s.app.Query(f)
		if e != nil {
			write(w, status(e), map[string]any{"error": e.Error(), "code": "INVALID_FILTER"})
			return
		}
		write(w, 200, out)
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if key != "" {
		if code, cached, ok := s.app.ReplayResult(key); ok {
			write(w, code, cached)
			return
		}
	}
	var q createReq
	if e := decode(r, &q); e != nil {
		write(w, 400, map[string]string{"error": e.Error()})
		return
	}
	c, cachedCode, cachedBody, e := s.app.CreateWithIdempotency(key, q.CaseNumber, q.SenderName, q.ReceiverName, q.HandoffWindowStart, q.HandoffWindowEnd)
	if c == nil && e == nil && cachedBody != nil {
		write(w, cachedCode, cachedBody)
		return
	}
	if e != nil {
		code := status(e)
		body := errorBody(e)
		if key != "" {
			s.app.RememberResult(key, code, body)
		}
		write(w, code, body)
		return
	}
	if key != "" {
		s.app.RememberResult(key, 201, c)
	}
	write(w, 201, c)
}
func parsePath(p string) (string, string) {
	x := strings.Split(strings.Trim(p, "/"), "/")
	if len(x) < 3 {
		return "", ""
	}
	return x[2], strings.Join(x[3:], "/")
}

type versionReq struct {
	ExpectedVersion int `json:"expectedVersion"`
}

func (s *Server) caseAction(w http.ResponseWriter, r *http.Request) {
	id, action := parsePath(r.URL.Path)
	if id == "" {
		http.NotFound(w, r)
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if key != "" {
		if code, cached, ok := s.app.ReplayResult(key); ok {
			write(w, code, cached)
			return
		}
	}
	if r.Method == "GET" && action == "" {
		c, e := s.app.Get(id)
		if e != nil {
			write(w, status(e), map[string]string{"error": e.Error()})
			return
		}
		c.ProbeCoverageLedger = domain.CalibrationLedger(c)
		c.WindowPhase, c.RemainingMinutes = domain.WindowTiming(c, time.Now().UTC())
		if sev := r.URL.Query().Get("severity"); sev != "" {
			cp := *c
			cp.Findings = nil
			for _, f := range c.Findings {
				if f.Severity == sev {
					cp.Findings = append(cp.Findings, f)
				}
			}
			c = &cp
		}
		write(w, 200, c)
		return
	}
	if r.Method == "GET" {
		switch action {
		case "manifest":
			entries, d, e := s.app.Manifest(id)
			if e != nil {
				write(w, status(e), map[string]string{"error": e.Error()})
				return
			}
			version := 0
			if c, err := s.app.Get(id); err == nil {
				version = c.Version
			}
			write(w, 200, map[string]any{"entries": entries, "manifestEntries": entries, "digest": d, "manifestDigest": d, "version": version})
			return
		case "audit":
			ev, e := s.app.Audit(id, r.URL.Query().Get("action"))
			if e != nil {
				write(w, status(e), map[string]string{"error": e.Error()})
				return
			}
			write(w, 200, map[string]any{"items": ev})
			return
		case "risk":
			c, e := s.app.Get(id)
			if e != nil {
				write(w, status(e), map[string]string{"error": e.Error()})
				return
			}
			filter, e := parseRiskFilter(r.URL.Query())
			if e != nil {
				write(w, 422, map[string]any{"error": e.Error(), "code": "INVALID_RISK_FILTER"})
				return
			}
			derived := *c
			derived.Findings = domain.FindingsForCase(c)
			for i := range derived.Findings {
				for _, previous := range c.Findings {
					if previous.ID == derived.Findings[i].ID && previous.Severity == derived.Findings[i].Severity && previous.DerivedReason == derived.Findings[i].DerivedReason && strings.Join(previous.EvidenceRefs, ",") == strings.Join(derived.Findings[i].EvidenceRefs, ",") {
						derived.Findings[i].Decision, derived.Findings[i].DecisionNote, derived.Findings[i].DecidedBy, derived.Findings[i].DecidedAt = previous.Decision, previous.DecisionNote, previous.DecidedBy, previous.DecidedAt
						break
					}
				}
			}
			findings := filterFindings(&derived, filter)
			summary := risk.Summarize(findings)
			severityCounts := map[string]int{"高": 0, "中": 0, "低": 0}
			totalDuration, maxDeviation := int64(0), 0.0
			highest := ""
			for _, f := range findings {
				severityCounts[f.Severity]++
				totalDuration += f.DurationSeconds
				if f.DeviationC > maxDeviation {
					maxDeviation = f.DeviationC
				}
				if risk.SeverityRank(f.Severity) > risk.SeverityRank(highest) {
					highest = f.Severity
				}
			}
			orderedSummary := make([]map[string]any, 0, len(summary))
			for _, kind := range []string{"超温", "读数断档", "采样稀疏", "校准失效", "封签变化"} {
				if item, ok := summary[kind]; ok {
					orderedSummary = append(orderedSummary, map[string]any{"kind": kind, "count": item.Count, "highestSeverity": item.HighestSeverity, "revisionNumbers": item.RevisionNumbers, "durationSeconds": item.DurationSeconds, "maxDeviationC": item.MaxDeviationC})
				}
			}
			diff := riskDiff(filterFindingsBaseline(c, filter), findings)
			allFingerprint := domain.FindingsFingerprint(derived.Findings)
			write(w, 200, map[string]any{"findings": findings, "summary": summary, "summaryItems": orderedSummary, "severityCounts": severityCounts, "count": len(findings), "highestSeverity": highest, "durationSeconds": totalDuration, "maxDeviationC": maxDeviation, "coverage": risk.EvidenceCoverage(c), "probeCoverageLedger": domain.CalibrationLedger(c), "fingerprint": domain.FindingsFingerprint(findings), "allFingerprint": allFingerprint, "confirmationToken": allFingerprint, "baselineFingerprint": c.ReviewFingerprint, "diff": diff, "remediationClosure": domain.RemediationClosureForCase(&derived), "remediationTasks": derived.RemediationTasks})
			return
		}
	}
	var e error
	var out any
	batchDecisions := false
	batchDecisionCount := 0
	switch action {
	case "conflict-review", "conflicts/review":
		var q struct {
			versionReq
			ConflictDigest, Decision, Reviewer, ReviewedBy, Operator string
			Conflicts                                                []domain.WindowConflict `json:"conflicts"`
		}
		if e = decode(r, &q); e == nil {
			if len(q.Conflicts) == 0 {
				if c, ge := s.app.Get(id); ge == nil {
					q.Conflicts = c.WindowConflicts
				}
			}
			if q.Reviewer == "" {
				if q.ReviewedBy != "" {
					q.Reviewer = q.ReviewedBy
				} else {
					q.Reviewer = q.Operator
				}
			}
			out, e = s.app.ReviewConflicts(id, q.ExpectedVersion, q.Conflicts, q.ConflictDigest, q.Decision, q.Reviewer)
		}
	case "probe-replace", "probes/replace", "probe-replacement", "probeReplacement":
		var q struct {
			versionReq
			OldProbeID       string    `json:"oldProbeId"`
			NewProbeID       string    `json:"newProbeId"`
			NewSerial        string    `json:"newSerialNumber"`
			NewCertificate   string    `json:"newCertificateRef"`
			CalibrationStart time.Time `json:"calibrationStart"`
			CalibrationEnd   time.Time `json:"calibrationEnd"`
			ReplacementAt    time.Time `json:"replacementAt"`
			domain.TemperatureProbe
		}
		if e = decode(r, &q); e == nil {
			if q.ReplacementAt.IsZero() {
				q.ReplacementAt = time.Now().UTC()
			}
			if q.ID == "" {
				q.ID = q.NewProbeID
			}
			if q.SerialNumber == "" {
				q.SerialNumber = q.NewSerial
			}
			if q.CertificateRef == "" {
				q.CertificateRef = q.NewCertificate
			}
			if q.CalibratedAt.IsZero() {
				q.CalibratedAt = q.CalibrationStart
			}
			if q.CalibrationExpiresAt.IsZero() {
				q.CalibrationExpiresAt = q.CalibrationEnd
			}
			out, e = s.app.ReplaceProbe(id, q.ExpectedVersion, q.OldProbeID, q.TemperatureProbe, q.ReplacementAt)
		}
	case "evidence-precheck", "evidence/precheck":
		var q struct {
			versionReq
			Items     []domain.TransportEvidenceRevision `json:"items"`
			Revisions []domain.TransportEvidenceRevision `json:"revisions"`
			domain.TransportEvidenceRevision
		}
		if e = decode(r, &q); e == nil {
			if len(q.Revisions) > 0 {
				q.Items = q.Revisions
			}
			if len(q.Items) == 0 {
				q.Items = []domain.TransportEvidenceRevision{q.TransportEvidenceRevision}
			}
			out, e = s.app.PrecheckEvidence(id, q.ExpectedVersion, q.Items)
		}
	case "containers":
		var q struct {
			versionReq
			domain.SampleContainer
			Items      []domain.SampleContainer  `json:"items"`
			Containers []domain.SampleContainer  `json:"containers"`
			Probes     []domain.TemperatureProbe `json:"probes"`
		}
		if e = decode(r, &q); e == nil {
			if len(q.Containers) > 0 {
				q.Items = q.Containers
			}
			if len(q.Items) > 0 || len(q.Probes) > 0 {
				e = validatePublicBasics(q.Items, q.Probes)
			}
			if e != nil {
				break
			}
			if len(q.Items) > 0 {
				out, e = s.app.RegisterBasics(id, q.ExpectedVersion, q.Items, q.Probes)
			} else {
				out, e = s.app.RegisterContainer(id, q.ExpectedVersion, q.SampleContainer)
			}
		}
	case "probes":
		var q struct {
			versionReq
			domain.TemperatureProbe
			Items      []domain.TemperatureProbe `json:"items"`
			Probes     []domain.TemperatureProbe `json:"probes"`
			Containers []domain.SampleContainer  `json:"containers"`
		}
		if e = decode(r, &q); e == nil {
			if len(q.Probes) > 0 {
				q.Items = q.Probes
			}
			if len(q.Containers) > 0 || len(q.Items) > 0 {
				e = validatePublicBasics(q.Containers, q.Items)
			}
			if e != nil {
				break
			}
			if len(q.Items) > 0 {
				out, e = s.app.RegisterBasics(id, q.ExpectedVersion, q.Containers, q.Items)
			} else {
				out, e = s.app.RegisterProbe(id, q.ExpectedVersion, q.TemperatureProbe)
			}
		}
	case "evidence":
		var q struct {
			versionReq
			PrecheckFingerprint string `json:"precheckFingerprint"`
			PrecheckToken       string `json:"precheckToken"`
			domain.TransportEvidenceRevision
			Items     []domain.TransportEvidenceRevision `json:"items"`
			Revisions []domain.TransportEvidenceRevision `json:"revisions"`
		}
		if e = decode(r, &q); e == nil {
			if q.PrecheckFingerprint == "" {
				q.PrecheckFingerprint = q.PrecheckToken
			}
			if len(q.Revisions) > 0 {
				q.Items = q.Revisions
			}
			checkItems := q.Items
			if len(checkItems) == 0 {
				checkItems = []domain.TransportEvidenceRevision{q.TransportEvidenceRevision}
			}
			e = validatePublicEvidence(checkItems)
			if e != nil {
				break
			}
			if len(q.Items) > 0 {
				out, e = s.app.AddRevisionsWithFingerprint(id, q.ExpectedVersion, q.Items, q.PrecheckFingerprint)
			} else {
				out, e = s.app.AddRevisionWithFingerprint(id, q.ExpectedVersion, q.TransportEvidenceRevision, q.PrecheckFingerprint)
			}
		}
	case "basics":
		var q struct {
			versionReq
			Containers []domain.SampleContainer  `json:"containers"`
			Probes     []domain.TemperatureProbe `json:"probes"`
		}
		if e = decode(r, &q); e == nil {
			e = validatePublicBasics(q.Containers, q.Probes)
			if e == nil {
				out, e = s.app.RegisterBasics(id, q.ExpectedVersion, q.Containers, q.Probes)
			}
		}
	case "submit":
		var q versionReq
		e = decode(r, &q)
		if e == nil {
			out, e = s.app.Submit(id, q.ExpectedVersion)
		}
	case "decisions":
		var q struct {
			versionReq
			FindingID, Decision, Note, Reviewer string
			Decisions                           []struct{ FindingID, Decision, Note, Reviewer string } `json:"decisions"`
		}
		if e = decode(r, &q); e == nil {
			if len(q.Decisions) > 0 {
				batchDecisions = true
				batchDecisionCount = len(q.Decisions)
				ds := make([]application.FindingDecision, len(q.Decisions))
				for i, d := range q.Decisions {
					ds[i] = application.FindingDecision{FindingID: d.FindingID, Decision: d.Decision, Note: d.Note, Reviewer: d.Reviewer}
				}
				out, e = s.app.DecideBatch(id, q.ExpectedVersion, ds)
			} else {
				out, e = s.app.Decide(id, q.ExpectedVersion, q.FindingID, q.Decision, q.Note, q.Reviewer)
			}
		}
	case "return":
		var q struct {
			versionReq
			Note, Reviewer   string
			Tasks            []domain.RemediationTask `json:"tasks"`
			RemediationTasks []domain.RemediationTask `json:"remediationTasks"`
			Items            []domain.RemediationTask `json:"items"`
		}
		if e = decode(r, &q); e == nil {
			if len(q.RemediationTasks) > 0 {
				q.Tasks = q.RemediationTasks
			}
			if len(q.Items) > 0 {
				q.Tasks = q.Items
			}
			if len(q.Tasks) > 0 {
				out, e = s.app.ReturnWithTasks(id, q.ExpectedVersion, q.Note, q.Reviewer, q.Tasks)
			} else {
				out, e = s.app.Return(id, q.ExpectedVersion, q.Note, q.Reviewer)
			}
		}
	case "approve":
		var q struct {
			versionReq
			Reviewer, Fingerprint, ReviewFingerprint string
		}
		if e = decode(r, &q); e == nil {
			if q.Fingerprint == "" {
				q.Fingerprint = q.ReviewFingerprint
			}
			out, e = s.app.ApproveWithFingerprint(id, q.ExpectedVersion, q.Reviewer, q.Fingerprint)
		}
	case "release":
		var q struct {
			versionReq
			Reviewer string
		}
		if e = decode(r, &q); e == nil {
			out, e = s.app.Issue(id, q.ExpectedVersion, q.Reviewer)
		}
	default:
		http.NotFound(w, r)
		return
	}
	if e != nil {
		body := errorBody(e)
		if action == "submit" {
			if c, getErr := s.app.Get(id); getErr == nil {
				body["coverage"] = risk.EvidenceCoverage(c)
				body["remediationClosure"] = domain.RemediationClosureForCase(c)
				body["remediationTasks"] = c.RemediationTasks
			}
		}
		write(w, status(e), body)
		return
	}
	if batchDecisions {
		if c, ok := out.(*domain.ColdChainCase); ok {
			meta := map[string]any{}
			b, _ := json.Marshal(c)
			_ = json.Unmarshal(b, &meta)
			remaining := 0
			undecided := make([]domain.RiskFinding, 0)
			for _, f := range c.Findings {
				if f.Decision == "" {
					remaining++
					undecided = append(undecided, f)
				}
			}
			sort.Slice(undecided, func(i, j int) bool { return undecided[i].ID < undecided[j].ID })
			meta["decidedCount"], meta["remainingUndecided"], meta["fingerprint"] = batchDecisionCount, remaining, domain.FindingsFingerprint(c.Findings)
			meta["undecided"] = undecided
			meta["undecidedFindings"] = undecided
			out = meta
		}
	}
	if key != "" {
		s.app.Remember(key, out)
	}
	write(w, 200, out)
}

func (s *Server) verify(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && (r.URL.Path == "/api/credentials/reviews" || r.URL.Query().Get("failureCode") != "") {
		write(w, 200, map[string]any{"items": s.app.CredentialReviews(r.URL.Query().Get("credentialNumber"), r.URL.Query().Get("failureCode"))})
		return
	}
	if r.Method == http.MethodPost {
		var q struct{ Input, CredentialNumber, BatchKey, BatchID, CheckDigest, FailureCode, Operator, Conclusion, Note string }
		if err := decode(r, &q); err != nil {
			write(w, 400, map[string]string{"error": err.Error()})
			return
		}
		if q.Input == "" && strings.HasSuffix(r.URL.Path, "/review") {
			q.Input = strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/credentials/"), "/review")
		}
		if q.Input == "" {
			q.Input = q.CredentialNumber
		}
		if q.BatchKey == "" {
			q.BatchKey = q.BatchID
		}
		if q.FailureCode != "" {
			check := s.app.VerifyCredentials([]string{strings.TrimSpace(q.Input)})
			if len(check.Items) == 0 || check.Items[0].Code != q.FailureCode {
				write(w, http.StatusConflict, map[string]any{"error": "失败代码与当前核验结果不一致", "code": "VERIFICATION_CHANGED"})
				return
			}
		}
		record, err := s.app.RecordCredentialReviewContext(r.Context(), q.Input, q.BatchKey, q.CheckDigest, q.Operator, q.Conclusion, q.Note)
		if err != nil {
			write(w, status(err), errorBody(err))
			return
		}
		write(w, 201, record)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := ""
	if r.URL.Path != "/api/credentials" {
		id = strings.TrimPrefix(r.URL.Path, "/api/credentials/")
	}
	if id == "" {
		id = r.URL.Query().Get("credentialNumber")
	}
	if id == "" {
		id = r.URL.Query().Get("caseID")
	}
	if id == "" {
		id = r.URL.Query().Get("caseId")
	}
	inputs := credentialInputs(r.URL.RawQuery)
	if id != "" && len(inputs) == 0 {
		inputs = []string{id}
	}
	if len(inputs) > 50 {
		write(w, 422, map[string]any{"error": "单次最多核验50项", "code": "BATCH_LIMIT_EXCEEDED"})
		return
	}
	if len(inputs) > 1 || len(inputs) == 1 && strings.Contains(firstValue(r.URL.Query(), "credentialNumber", "caseID", "caseId"), ",") {
		key := r.Header.Get("Idempotency-Key")
		result, err := s.app.VerifyCredentialsRecorded(inputs, key)
		if err != nil {
			write(w, status(err), errorBody(err))
			return
		}
		write(w, 200, result)
		return
	}
	if len(inputs) == 1 {
		id = inputs[0]
	}
	result, e := s.app.VerifyCredentialDetailed(id)
	if e != nil {
		code := "CREDENTIAL_NOT_FOUND"
		if errors.Is(e, domain.ErrInvalidState) {
			code = "CASE_NOT_RELEASED"
		}
		write(w, status(e), map[string]string{"error": e.Error(), "code": code})
		return
	}
	caseNumber, caseStatus := "", result.CaseStatus
	if result.Credential != nil {
		if caseInfo, err := s.app.Get(result.Credential.CaseID); err == nil {
			caseNumber, caseStatus = caseInfo.CaseNumber, caseInfo.Status
		}
	}
	code := ""
	for _, check := range result.Checks {
		if !check.Passed && check.Code != "" {
			code = check.Code
			break
		}
	}
	if result.Credential == nil && result.CaseStatus != domain.StatusReleased {
		code = "CASE_NOT_RELEASED"
	}
	write(w, 200, map[string]any{"valid": result.Valid, "code": code, "checks": result.Checks, "credential": result.Credential, "caseNumber": caseNumber, "status": caseStatus, "caseStatus": result.CaseStatus})
}

func (s *Server) verificationRecords(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	credential := r.URL.Query().Get("credentialNumber")
	if credential == "" {
		credential = r.URL.Query().Get("credentialId")
	}
	write(w, 200, map[string]any{"items": s.app.CredentialReviews(credential, r.URL.Query().Get("failureCode"))})
}

func credentialInputs(rawQuery string) []string {
	out := make([]string, 0)
	for _, part := range strings.Split(rawQuery, "&") {
		pair := strings.SplitN(part, "=", 2)
		if len(pair) != 2 {
			continue
		}
		name, err := url.QueryUnescape(pair[0])
		if err != nil || name != "credentialNumber" && name != "caseID" && name != "caseId" {
			continue
		}
		value, err := url.QueryUnescape(pair[1])
		if err != nil {
			continue
		}
		for _, item := range strings.Split(value, ",") {
			if normalized := strings.TrimSpace(item); normalized != "" {
				out = append(out, normalized)
			}
		}
	}
	return out
}

type riskFilter struct {
	kind, severity string
	revision       int
	start, end     *time.Time
}

func parseRiskFilter(q url.Values) (riskFilter, error) {
	f := riskFilter{kind: q.Get("kind"), severity: q.Get("severity")}
	if f.severity != "" && risk.SeverityRank(f.severity) == 0 {
		return f, domain.ErrInvalid
	}
	if v := firstValue(q, "revisionNumber", "revision"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return f, domain.ErrInvalid
		}
		f.revision = n
	}
	parse := func(names ...string) (*time.Time, error) {
		for _, name := range names {
			if v := q.Get(name); v != "" {
				t, err := time.Parse(time.RFC3339, v)
				if err != nil {
					return nil, domain.ErrInvalid
				}
				return &t, nil
			}
		}
		return nil, nil
	}
	var err error
	if f.start, err = parse("startAt", "start", "from", "startTime"); err != nil {
		return f, err
	}
	if f.end, err = parse("endAt", "end", "to", "endTime"); err != nil {
		return f, err
	}
	if f.start != nil && f.end != nil && f.end.Before(*f.start) {
		return f, domain.ErrInvalid
	}
	return f, nil
}

func firstValue(q url.Values, names ...string) string {
	for _, name := range names {
		if value := q.Get(name); value != "" {
			return value
		}
	}
	return ""
}

func findingWindow(c *domain.ColdChainCase, f domain.RiskFinding) (time.Time, time.Time) {
	if f.StartAt != nil && f.EndAt != nil {
		return *f.StartAt, *f.EndAt
	}
	for _, r := range c.Revisions {
		if r.RevisionNumber == f.RevisionNumber {
			return r.SegmentStart, r.SegmentEnd
		}
	}
	return time.Time{}, time.Time{}
}

func filterFindings(c *domain.ColdChainCase, f riskFilter) []domain.RiskFinding {
	out := make([]domain.RiskFinding, 0, len(c.Findings))
	for _, item := range c.Findings {
		if f.kind != "" && item.Kind != f.kind || f.severity != "" && item.Severity != f.severity || f.revision > 0 && item.RevisionNumber != f.revision {
			continue
		}
		start, end := findingWindow(c, item)
		if f.start != nil && !end.IsZero() && end.Before(*f.start) {
			continue
		}
		if f.end != nil && !start.IsZero() && start.After(*f.end) {
			continue
		}
		item.EvidenceRefs = intersectEvidenceRefs(c, item, f)
		item.Context = intersectReadingContext(item.Context, f)
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func intersectReadingContext(context *domain.RiskReadingContext, f riskFilter) *domain.RiskReadingContext {
	if context == nil || f.start == nil && f.end == nil {
		return context
	}
	clone := *context
	inside := func(reading *domain.TemperatureReading) *domain.TemperatureReading {
		if reading == nil || f.start != nil && reading.At.Before(*f.start) || f.end != nil && reading.At.After(*f.end) {
			return nil
		}
		copy := *reading
		return &copy
	}
	clone.TriggerReading = inside(context.TriggerReading)
	clone.PreviousReading = inside(context.PreviousReading)
	clone.NextReading = inside(context.NextReading)
	return &clone
}

func intersectEvidenceRefs(c *domain.ColdChainCase, finding domain.RiskFinding, f riskFilter) []string {
	if f.start == nil && f.end == nil {
		return append([]string(nil), finding.EvidenceRefs...)
	}
	refs := make([]string, 0, len(finding.EvidenceRefs))
	for _, ref := range finding.EvidenceRefs {
		atStart, atEnd := time.Time{}, time.Time{}
		parts := strings.SplitN(ref, "#", 2)
		for _, rev := range c.Revisions {
			if rev.ID != parts[0] {
				continue
			}
			atStart, atEnd = rev.SegmentStart, rev.SegmentEnd
			if len(parts) == 2 {
				if index, err := strconv.Atoi(parts[1]); err == nil && index >= 0 && index < len(rev.Readings) {
					atStart, atEnd = rev.Readings[index].At, rev.Readings[index].At
				}
			}
			break
		}
		if f.start != nil && !atEnd.IsZero() && atEnd.Before(*f.start) {
			continue
		}
		if f.end != nil && !atStart.IsZero() && atStart.After(*f.end) {
			continue
		}
		refs = append(refs, ref)
	}
	return refs
}

func filterFindingsBaseline(c *domain.ColdChainCase, f riskFilter) []domain.RiskFinding {
	clone := *c
	clone.Findings = c.ReviewBaseline
	return filterFindings(&clone, f)
}

func riskDiff(baseline, current []domain.RiskFinding) map[string]any {
	old, now := map[string]domain.RiskFinding{}, map[string]domain.RiskFinding{}
	for _, f := range baseline {
		old[f.ID] = f
	}
	for _, f := range current {
		now[f.ID] = f
	}
	added, removed, kept := []string{}, []string{}, []string{}
	changed, severityChanged, evidenceChanged := []map[string]any{}, []map[string]any{}, []map[string]any{}
	for id, f := range now {
		previous, ok := old[id]
		if !ok {
			added = append(added, id)
			continue
		}
		severityDiff := previous.Severity != f.Severity
		evidenceDiff := strings.Join(previous.EvidenceRefs, "\n") != strings.Join(f.EvidenceRefs, "\n") || previous.ContextID != f.ContextID
		var changedItem map[string]any
		if severityDiff {
			item := map[string]any{"id": id, "from": previous.Severity, "to": f.Severity}
			severityChanged = append(severityChanged, item)
			changedItem = item
		}
		if evidenceDiff {
			item := map[string]any{"id": id, "from": previous.EvidenceRefs, "to": f.EvidenceRefs}
			evidenceChanged = append(evidenceChanged, item)
			if changedItem == nil {
				changedItem = map[string]any{"id": id}
			}
			changedItem["evidenceFrom"], changedItem["evidenceTo"] = previous.EvidenceRefs, f.EvidenceRefs
		}
		if changedItem != nil {
			changed = append(changed, changedItem)
		}
		if !severityDiff && !evidenceDiff {
			kept = append(kept, id)
		}
	}
	for id := range old {
		if _, ok := now[id]; !ok {
			removed = append(removed, id)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	sort.Strings(kept)
	sort.Slice(changed, func(i, j int) bool { return changed[i]["id"].(string) < changed[j]["id"].(string) })
	sort.Slice(severityChanged, func(i, j int) bool { return severityChanged[i]["id"].(string) < severityChanged[j]["id"].(string) })
	sort.Slice(evidenceChanged, func(i, j int) bool { return evidenceChanged[i]["id"].(string) < evidenceChanged[j]["id"].(string) })
	return map[string]any{"added": added, "removed": removed, "kept": kept, "changed": changed, "severityChanged": severityChanged, "evidenceChanged": evidenceChanged}
}

func parseFilter(q url.Values) (domain.CaseFilter, error) {
	first := func(names ...string) string {
		for _, n := range names {
			if v := q.Get(n); v != "" {
				return v
			}
		}
		return ""
	}
	f := domain.CaseFilter{Status: q.Get("status"), CaseNumber: first("caseNumber", "keyword"), Sender: first("sender", "senderName", "senderKeyword"), Receiver: first("receiver", "receiverName", "receiverKeyword"), WindowPhase: first("windowPhase", "phase")}
	if !domain.ValidCaseStatus(f.Status) {
		return f, domain.ErrInvalid
	}
	if f.WindowPhase != "" && f.WindowPhase != domain.WindowPhasePending && f.WindowPhase != domain.WindowPhaseActive && f.WindowPhase != domain.WindowPhaseEnded {
		return f, domain.ErrInvalid
	}
	parse := func(name string) (*time.Time, error) {
		v := q.Get(name)
		if v == "" {
			return nil, nil
		}
		t, e := time.Parse(time.RFC3339, v)
		if e != nil {
			return nil, domain.ErrInvalid
		}
		return &t, nil
	}
	var e error
	startName := "handoffStart"
	if q.Get(startName) == "" {
		startName = "handoffWindowStart"
	}
	endName := "handoffEnd"
	if q.Get(endName) == "" {
		endName = "handoffWindowEnd"
	}
	if f.HandoffStart, e = parse(startName); e != nil {
		return f, e
	}
	if f.HandoffEnd, e = parse(endName); e != nil {
		return f, e
	}
	if f.HandoffStart != nil && f.HandoffEnd != nil && f.HandoffEnd.Before(*f.HandoffStart) {
		return f, domain.ErrInvalid
	}
	parseInt := func(name string, fallback int) (int, error) {
		v := q.Get(name)
		if v == "" {
			return fallback, nil
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, domain.ErrInvalid
		}
		return n, nil
	}
	pageName := "page"
	if q.Get(pageName) == "" && q.Get("pageNumber") != "" {
		pageName = "pageNumber"
	}
	if f.Page, e = parseInt(pageName, 1); e != nil {
		return f, e
	}
	pageSizeName := "pageSize"
	if q.Get(pageSizeName) == "" && q.Get("size") != "" {
		pageSizeName = "size"
	}
	if f.PageSize, e = parseInt(pageSizeName, 50); e != nil {
		return f, e
	}
	if f.Page < 1 || f.PageSize < 1 || f.PageSize > 100 {
		return f, domain.ErrInvalid
	}
	if due := first("dueWithinMinutes", "urgencyMinutes"); due != "" {
		n, err := strconv.Atoi(due)
		if err != nil || n < 0 {
			return f, domain.ErrInvalid
		}
		f.DueWithinMinutes = &n
	}
	return f, nil
}
