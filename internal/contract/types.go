package contract

type Fact struct {
	ID         string
	Value      string
	Source     string
	Confidence string
}

type Profile struct {
	ID         string
	Selectable bool
	Reason     string
	Title      string
}

type Describe struct {
	Format   string
	Facts    []Fact
	Profiles []Profile
}

func (d Describe) Fact(id string) (Fact, bool) {
	for _, fact := range d.Facts {
		if fact.ID == id {
			return fact, true
		}
	}
	return Fact{}, false
}

func (d Describe) Profile(id string) (Profile, bool) {
	for _, profile := range d.Profiles {
		if profile.ID == id {
			return profile, true
		}
	}
	return Profile{}, false
}

type PlanMeta struct {
	Profile      string
	Ref          string
	ConfigHome   string
	CheckoutPath string
	ReceiptPath  string
	SkipZshrc    bool
}

type Checkout struct {
	Kind         string
	Head         string
	CurrentRef   string
	Origin       string
	RequestedRef string
}

type Target struct {
	ID        string
	Path      string
	Kind      string
	Expected  string
	Mode      string
	Content   []byte
	BlockHash string
	Changed   bool
}

type Operation struct {
	ID            string
	Phase         string
	Kind          string
	Summary       string
	Interruptible bool
}

type Warning struct {
	ID          string
	Severity    string
	Summary     string
	Remediation string
}

type Plan struct {
	Format     string
	ID         string
	Meta       PlanMeta
	Checkout   Checkout
	Targets    []Target
	Operations []Operation
	Warnings   []Warning
}

func (p Plan) HasContentChanges() bool {
	for _, target := range p.Targets {
		if target.Changed {
			return true
		}
	}
	return false
}

type ResultOperation struct {
	ID     string
	Status string
	Detail string
}

type ResultError struct {
	Code      string
	Operation string
	Detail    string
}

type Result struct {
	Format      string
	PlanID      string
	Phase       string
	Status      string
	Operations  []ResultOperation
	Error       *ResultError
	ReceiptPath string
}
