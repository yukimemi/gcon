package cmd

import (
	"strings"

	"github.com/yukimemi/core"
)

func init() {
	funcs["Exec"] = (*Gcon).Exec
}

// ArgsExec is Exec func args.
type ArgsExec struct {
	Cmds []Cmd `validate:"required,dive,required"`

	ArgsDef
}

// Cmd is command exec list.
type Cmd struct {
	Cmd string `validate:"required"`
	Arg string
	IP  string
	Enc string
}

// Exec execute process.
func (g *Gcon) Exec(args Args) (*TaskInfo, error) {

	a := &ArgsExec{}
	err := g.ParseArgs(args, a)
	if err != nil {
		return nil, err
	}

	// Execute cmd list.
	for _, cmd := range a.Cmds {
		c := core.Cmd{CmdLine: cmd.Cmd + " " + cmd.Arg}
		err := c.CmdRun()
		if err != nil {
			return nil, err
		}
		g.Infof("Cmd:      [%v]", c.Cmd.Args[0])
		g.Infof("Arg:      [%v]", strings.Join(c.Cmd.Args[1:], " "))
		g.Infof("ExitCode: [%v]", c.ExitCode)
		g.Infof("Stdout:   [%v]", c.Stdout.String())
		g.Infof("Stderr:   [%v]", c.Stderr.String())
	}

	// Get target async func.
	return nil, nil
}
