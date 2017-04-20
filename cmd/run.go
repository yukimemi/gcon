package cmd

func init() {
	funcs["Run"] = (*Gcon).Run
}

// ArgsRun is Run func args.
type ArgsRun struct {
	Targets []Target `require:"true"`
}

// Target is task info.
type Target struct {
	ID   string `require:"true"`
	File string `require:"false"`
}

// Run output msg.
func (g *Gcon) Run(args Args) (TaskInfo, error) {

	a := &ArgsRun{}
	err := ParseArgs(args, a)
	if err != nil {
		return TaskInfo{}, err
	}

	for _, target := range a.Targets {
		ti := TaskInfo{
			ID:      target.ID,
			Path:    target.File,
			ProType: Normal,
		}
		if ti.Path == "" {
			ti.Path = g.Ti.Path
		}
		err := g.Engine(ti)
		if err != nil {
			return TaskInfo{}, err
		}
	}

	return TaskInfo{}, nil
}
