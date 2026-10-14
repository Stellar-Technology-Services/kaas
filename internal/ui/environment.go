package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Stellar-Technology-Services/kaas/internal/sandbox/contract"
	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

// EnvironmentDTO is boat's "environment" view of a sandbox: what the machine
// may use, as recorded in the redacted contract. Secret values never appear —
// the contract type has no value fields.
type EnvironmentDTO struct {
	Hash         string
	Sources      []contract.WorkspaceSourceContract
	Services     []contract.ServiceContract
	SecretMode   string
	SecretKeys   []string
	EgressMode   string
	EgressDeny   bool
	EgressHosts  []egressHostDTO
	Drift        []contract.ContractChange
	DriftError   string
	DriftChecked bool
}

type egressHostDTO struct {
	Host       string
	Credential string // key name + injection type, or ""
}

func environmentFor(snap *state.ContractSnapshot, statePath, repoPath string) *EnvironmentDTO {
	if snap == nil {
		return nil
	}
	c, err := contract.ParseJSON([]byte(snap.ContractJSON))
	if err != nil {
		return &EnvironmentDTO{Hash: snap.ContractHash, DriftError: "contract JSON unreadable: " + err.Error()}
	}
	env := &EnvironmentDTO{
		Hash:       snap.ContractHash,
		Sources:    c.Workspace.Sources,
		Services:   c.Workspace.Services,
		SecretMode: c.Secrets.Mode,
		EgressMode: c.Egress.Mode,
		EgressDeny: c.Egress.Default == "deny",
	}
	for _, k := range c.Secrets.Keys {
		env.SecretKeys = append(env.SecretKeys, k.Key)
	}
	for _, r := range c.Egress.Rules {
		cred := ""
		if r.CredentialKey != "" {
			cred = r.CredentialKey + " (" + r.Injection + ")"
		}
		env.EgressHosts = append(env.EgressHosts, egressHostDTO{Host: r.Host, Credential: cred})
	}
	if repoPath != "" {
		env.DriftChecked = true
		changes, derr := contract.DriftFromProject(statePath, repoPath, c)
		if derr != nil {
			env.DriftError = derr.Error()
		} else {
			env.Drift = changes
		}
	}
	return env
}

// PhaseLogDTO describes one captured readiness-phase log.
type PhaseLogDTO struct {
	Phase    string
	Path     string
	Size     int64
	Modified time.Time
	Tail     []string
}

// phaseLogsFor lists <logsDir>/<sandbox>/*.log newest first; the first (most
// recent) entry carries a tail so a failure is visible without a click.
func phaseLogsFor(logsDir, sandbox string) []PhaseLogDTO {
	if logsDir == "" || sandbox == "" {
		return nil
	}
	dir := filepath.Join(logsDir, sandbox)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []PhaseLogDTO
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, PhaseLogDTO{
			Phase:    strings.TrimSuffix(e.Name(), ".log"),
			Path:     filepath.Join(dir, e.Name()),
			Size:     info.Size(),
			Modified: info.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Modified.After(out[j].Modified) })
	if len(out) > 0 {
		if tail, err := tailFile(out[0].Path, 25); err == nil {
			out[0].Tail = tail
		}
	}
	return out
}

// phaseLogPath validates a phase name from the URL before reading a file.
func phaseLogPath(logsDir, sandbox, phase string) (string, bool) {
	if logsDir == "" || sandbox == "" || phase == "" || strings.ContainsAny(phase, "/\\.") || strings.ContainsAny(sandbox, "/\\.") {
		return "", false
	}
	return filepath.Join(logsDir, sandbox, phase+".log"), true
}
