package cmd

func init() {
	funcs["Log"] = (*Gcon).Log
}

// ArgsLog is Log func args.
type ArgsLog struct {
	Msg    string `validate:"required"`
	Stdout bool

	ArgsDef
}

// Log output msg.
func (g *Gcon) Log(args Args) (*TaskInfo, error) {

	a := &ArgsLog{}
	err := g.ParseArgs(args, a)
	if err != nil {
		return nil, err
	}

	return g.log(*a)
}

func (g *Gcon) log(a ArgsLog) (*TaskInfo, error) {

	if a.Stdout {
		// fmt.Println(a.Msg)
	}

	g.Infof(a.Msg)

	return nil, nil
}
