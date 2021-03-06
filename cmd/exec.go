package cmd

import (
	"strings"

	"github.com/yukimemi/core"
	"golang.org/x/text/encoding/japanese"
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
	Cmd     string `validate:"required"`
	Arg     string
	IP      string
	Sjis    bool
	Results []Result
	Other   Target `validate:"-"`
}

// Result is exit code pattern task.
type Result struct {
	Code int
	Target
}

// Exec execute process.
func (g *Gcon) Exec(args Args) (*TaskInfo, error) {

	a := &ArgsExec{}
	err := g.ParseArgs(args, a)
	if err != nil {
		return nil, err
	}

	return g.exec(*a)
}

func (g *Gcon) exec(a ArgsExec) (*TaskInfo, error) {

	// Execute cmd list.
	for _, cmd := range a.Cmds {
		c, err := core.NewCmd(cmd.Cmd + " " + cmd.Arg)
		if err != nil {
			return nil, err
		}
		if cmd.Sjis {
			c.StdoutEnc = japanese.ShiftJIS.NewDecoder()
		}
		soScanner, err := c.StdoutScanner()
		if err != nil {
			return nil, err
		}
		seScanner, err := c.StderrScanner()
		if err != nil {
			return nil, err
		}

		err = c.CmdStart()
		if err == nil {
			// Asynchronous output log.
			c.Wg.Add(1)
			go func() {
				defer c.Wg.Done()
				for soScanner.Scan() {
					g.Infof(soScanner.Text())
				}
			}()
			c.Wg.Add(1)
			go func() {
				defer c.Wg.Done()
				for seScanner.Scan() {
					g.Errorf(seScanner.Text())
				}
			}()
			c.CmdWait()
		}

		g.Infof("Cmd  : [%v]", c.Cmd.Args[0])
		g.Infof("Arg  : [%v]", strings.Join(c.Cmd.Args[1:], " "))
		g.Infof("Code : [%v]", c.ExitCode)

		target := new(Target)
		for _, result := range cmd.Results {
			g.Infof("id: [%v] code: [%v]", result.ID, result.Code)
			if c.ExitCode == result.Code && result.ID != "" {
				*target = result.Target
				break
			}
		}

		if target.ID == "" && cmd.Other.ID != "" {
			*target = cmd.Other
		}

		if target.ID != "" {
			// Run result or other target.
			ar := ArgsRun{Targets: []Target{*target}}
			ti, err := g.run(ar)
			if err != nil {
				return ti, err
			}
		}

	}

	// Get target async func.
	return nil, nil
}
