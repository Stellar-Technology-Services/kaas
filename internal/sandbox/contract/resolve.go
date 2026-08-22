package contract

import (
	"context"
	"fmt"
	"reflect"
	"strings"
)

// Layer is a partial contract contribution from one authority level.
// Nil pointers mean the layer does not set that section.
type Layer struct {
	Resources *Resources
	Workspace *WorkspaceContract
	Secrets   *SecretContract
	Egress    *EgressContract
	Source    string
}

// Input is the authority stack for Resolve.
// Precedence: BuiltIn < User < Project < Env < Flags.
type Input struct {
	BuiltIn Layer
	User    Layer
	Project Layer
	Env     Layer
	Flags   Layer
	// Persisted is the stored contract. Nil means no contract row exists yet.
	Persisted *Contract
	// Adopt requests replacing Persisted with the newly resolved stack (D3).
	Adopt bool
	// Reconcile is the explicit change-accept path (same authority as Adopt).
	Reconcile bool
}

// Resolved is the outcome of Resolve.
type Resolved struct {
	Contract Contract
	Plan     ExecutionPlan
	Changes  []ContractChange
	Adopted  bool
}

// ExecutionPlan describes adapter work implied by the contract.
type ExecutionPlan struct {
	ApplyResources    bool
	Provision         bool
	DeploySecrets     bool
	ConfigureEgress   bool
	CheckoutWorkspace bool
	ValidateVarlock   bool
	RunSetup          bool
}

// ContractChange is a human-readable field delta between two contracts.
type ContractChange struct {
	Path string `json:"path"`
	From string `json:"from"`
	To   string `json:"to"`
}

// Resolver resolves an effective sandbox contract from layered inputs.
type Resolver struct{}

// Resolve applies persisted authority (D3): an existing contract is the sole
// authority unless adopt/reconcile is requested. When no contract exists yet,
// or an explicit adopt/reconcile is requested, the merged stack is adopted.
func (Resolver) Resolve(_ context.Context, in Input) (Resolved, error) {
	stack, err := mergeStack(in)
	if err != nil {
		return Resolved{}, err
	}
	stack, err = stack.WithHash()
	if err != nil {
		return Resolved{}, err
	}

	explicit := in.Adopt || in.Reconcile

	// A persisted contract is the sole authority unless adopt/reconcile.
	if in.Persisted != nil && !explicit {
		persisted := *in.Persisted
		persisted.Normalize()
		if persisted.ContentHash == "" {
			persisted, err = persisted.WithHash()
			if err != nil {
				return Resolved{}, err
			}
		}
		changes := Compare(&persisted, &stack)
		return Resolved{
			Contract: persisted,
			Plan:     planFor(persisted),
			Changes:  changes,
			Adopted:  false,
		}, nil
	}

	// No persisted contract, or an explicit adopt/reconcile → adopt the stack.
	var changes []ContractChange
	if in.Persisted != nil {
		changes = Compare(in.Persisted, &stack)
	}
	return Resolved{
		Contract: stack,
		Plan:     planFor(stack),
		Changes:  changes,
		Adopted:  true,
	}, nil
}

func mergeStack(in Input) (Contract, error) {
	var c Contract
	c.SchemaVersion = CurrentSchemaVersion
	var sources []string
	configSource := ""

	apply := func(layer Layer) {
		if layer.Source != "" {
			sources = append(sources, layer.Source)
			if configSource == "" || layer.Source != "built-in" && layer.Source != "flags" && layer.Source != "env" {
				// Prefer project/user paths as ConfigSource.
				if strings.Contains(layer.Source, "kaas.yaml") || strings.HasPrefix(layer.Source, "/") || strings.HasPrefix(layer.Source, "~") {
					configSource = layer.Source
				}
			}
		}
		if layer.Resources != nil {
			c.Resources = mergeResources(c.Resources, *layer.Resources)
		}
		if layer.Workspace != nil {
			c.Workspace = mergeWorkspace(c.Workspace, *layer.Workspace)
		}
		if layer.Secrets != nil {
			c.Secrets = mergeSecrets(c.Secrets, *layer.Secrets)
		}
		if layer.Egress != nil {
			c.Egress = mergeEgress(c.Egress, *layer.Egress)
		}
	}

	apply(in.BuiltIn)
	apply(in.User)
	apply(in.Project)
	apply(in.Env)
	apply(in.Flags)

	c.Provenance = Provenance{
		Sources:      uniquePreserve(sources),
		ConfigSource: configSource,
	}
	c.Normalize()
	if c.Resources.CPUs < 1 {
		return Contract{}, fmt.Errorf("resolved resources.cpus must be >= 1")
	}
	if c.Resources.MemoryGiB < 1 {
		return Contract{}, fmt.Errorf("resolved resources.memory_gib must be >= 1")
	}
	if c.Resources.DiskGiB < 1 {
		return Contract{}, fmt.Errorf("resolved resources.disk_gib must be >= 1")
	}
	if strings.TrimSpace(c.Resources.Arch) == "" {
		return Contract{}, fmt.Errorf("resolved resources.arch is required")
	}
	return c, nil
}

func mergeResources(base, over Resources) Resources {
	out := base
	if over.CPUs > 0 {
		out.CPUs = over.CPUs
	}
	if over.MemoryGiB > 0 {
		out.MemoryGiB = over.MemoryGiB
	}
	if over.DiskGiB > 0 {
		out.DiskGiB = over.DiskGiB
	}
	if over.Arch != "" {
		out.Arch = over.Arch
	}
	if over.SSHPort > 0 {
		out.SSHPort = over.SSHPort
		out.SSHPortPolicy = ""
	} else if over.SSHPortPolicy != "" {
		out.SSHPortPolicy = over.SSHPortPolicy
	}
	return out
}

func mergeWorkspace(base, over WorkspaceContract) WorkspaceContract {
	out := base
	if over.GitAuth != "" {
		out.GitAuth = over.GitAuth
	}
	if over.Varlock.Schema != "" {
		out.Varlock.Schema = over.Varlock.Schema
	}
	if over.Varlock.ValidateCommand != "" {
		out.Varlock.ValidateCommand = over.Varlock.ValidateCommand
	}
	if over.Setup.Script != "" {
		out.Setup.Script = over.Setup.Script
	}
	// Sources replace when the overlay declares any source list.
	if len(over.Sources) > 0 {
		out.Sources = append([]WorkspaceSourceContract(nil), over.Sources...)
	}
	// Services replace when the overlay declares any service list (including empty
	// via an explicit non-nil slice from config mapping).
	if over.Services != nil {
		out.Services = append([]ServiceContract(nil), over.Services...)
	}
	out.normalizeSources()
	out.normalizeServices()
	return out
}

func mergeSecrets(base, over SecretContract) SecretContract {
	out := base
	if over.Mode != "" {
		out.Mode = over.Mode
	}
	if over.Keys != nil {
		out.Keys = append([]SecretReference(nil), over.Keys...)
	}
	return out
}

func mergeEgress(base, over EgressContract) EgressContract {
	out := over
	if out.SchemaVersion == 0 {
		out.SchemaVersion = base.SchemaVersion
	}
	// Full replace of rules when overlay sets egress (project owns policy).
	if over.Mode == "" && over.Default == "" && len(over.Rules) == 0 && over.SchemaVersion == 0 {
		return base
	}
	return out
}

func planFor(c Contract) ExecutionPlan {
	return ExecutionPlan{
		ApplyResources:    c.Resources.CPUs > 0,
		Provision:         true,
		DeploySecrets:     c.Secrets.Mode == SecretModeOpen && len(c.Secrets.Keys) > 0,
		ConfigureEgress:   c.Egress.Mode != "" && c.Egress.Mode != "open",
		CheckoutWorkspace: len(c.Workspace.Sources) > 0,
		ValidateVarlock:   c.Workspace.Varlock.Schema != "",
		RunSetup:          c.Workspace.Setup.Script != "",
	}
}

// Compare returns field-level changes from persisted to resolved.
// A nil side is treated as empty.
func Compare(persisted, resolved *Contract) []ContractChange {
	var a, b Contract
	if persisted != nil {
		a = *persisted
		a.Normalize()
		a.ContentHash = ""
		a.Egress.PolicyHash = ""
	}
	if resolved != nil {
		b = *resolved
		b.Normalize()
		b.ContentHash = ""
		b.Egress.PolicyHash = ""
	}
	var changes []ContractChange
	diffValue("", reflect.ValueOf(a), reflect.ValueOf(b), &changes)
	return changes
}

func diffValue(path string, a, b reflect.Value, out *[]ContractChange) {
	if !a.IsValid() || !b.IsValid() {
		return
	}
	if a.Kind() == reflect.Pointer {
		if a.IsNil() && b.IsNil() {
			return
		}
		if a.IsNil() || b.IsNil() {
			*out = append(*out, ContractChange{Path: path, From: fmt.Sprint(a.Interface()), To: fmt.Sprint(b.Interface())})
			return
		}
		diffValue(path, a.Elem(), b.Elem(), out)
		return
	}
	switch a.Kind() {
	case reflect.Struct:
		t := a.Type()
		for i := 0; i < a.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" { // unexported
				continue
			}
			name := f.Name
			if tag := f.Tag.Get("json"); tag != "" && tag != "-" {
				name = strings.Split(tag, ",")[0]
			}
			if name == "hash" || name == "policy_hash" || name == "provenance" {
				continue
			}
			child := name
			if path != "" {
				child = path + "." + name
			}
			diffValue(child, a.Field(i), b.Field(i), out)
		}
	case reflect.Slice:
		if a.Len() != b.Len() || !reflect.DeepEqual(a.Interface(), b.Interface()) {
			*out = append(*out, ContractChange{
				Path: path,
				From: fmt.Sprint(a.Interface()),
				To:   fmt.Sprint(b.Interface()),
			})
		}
	default:
		if !reflect.DeepEqual(a.Interface(), b.Interface()) {
			*out = append(*out, ContractChange{
				Path: path,
				From: fmt.Sprint(a.Interface()),
				To:   fmt.Sprint(b.Interface()),
			})
		}
	}
}
