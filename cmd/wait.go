package cmd

func init() {
	funcs["Wait"] = (*Gcon).Wait
}

// ArgsWait is Wait func args.
type ArgsWait struct {
	Targets []string `validate:"required"`

	ArgsDef
}

// Wait wait asynchronous func.
func (g *Gcon) Wait(args Args) (*TaskInfo, error) {

	a := &ArgsWait{}
	err := g.ParseArgs(args, a)
	if err != nil {
		return nil, err
	}

	// Get target async func.
	afs := make([]*FuncInfo, 0)
	asyncMu.Lock()
	for _, af := range asyncFis {
		for _, target := range a.Targets {
			if target == af.Async {
				afs = append(afs, af)
			}
		}
	}
	asyncMu.Unlock()

	// Wait async funcs.
	for _, af := range afs {
		g.Infof("Waiting func ID: [%v] Name: [%v] Async: [%v]", af.ID, af.Name, af.Async)
		af.Wg.Wait()
	}

	return nil, nil
}
