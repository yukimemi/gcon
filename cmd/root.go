// Copyright © 2017 yukimemi <yukimemi@gmail.com>
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/yukimemi/file"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
)

const (
	// ExitOK is success exit code.
	ExitOK = iota
	// ExitNG is error exit code.
	ExitNG
)

const (
	// Normal is normal process.
	Normal ProcessType = iota
	// Error is error process.
	Error
	// Final is final process.
	Final
)

// Init task id.
const Init = "Init"

// ProcessType is type of process.
type ProcessType int

// CfgInfo is config file information.
type CfgInfo struct {
	Path string
	Init bool
	Cfg
}

// Cfg is gcon config file struct.
type Cfg struct {
	Tasks []Task     `mapstructure:"tasks" yaml:"tasks"`
	Log   zap.Config `mapstructure:"log" yaml:"log"`
}

// CfgInfos is Array of CfgInfo.
type CfgInfos []CfgInfo

// Task is execute task information.
type Task struct {
	// ID is task id.
	ID     string  `mapstructure:"id" yaml:"id"`
	Normal Process `mapstructure:"normal" yaml:"normal"`
	Error  Process `mapstructure:"error" yaml:"error"`
	Final  Process `mapstructure:"final" yaml:"final"`
}

// TaskInfo is task file and task id.
type TaskInfo struct {
	Path    string
	ID      string
	ProType ProcessType
}

// Func is function struct.
type Func struct {
	Name string `mapstructure:"func" yaml:"func"`
	Args Args   `mapstructure:"args" yaml:"args"`
}

// Args is func arg.
type Args map[interface{}]interface{}

// Process is array of Func.
type Process []Func

// Funcs is func map.
type Funcs map[string]func(*Gcon, Args) (TaskInfo, error)

// Store store string data.
type Store map[string]interface{}

// Logger is log struct.
type Logger struct {
	Spin  *spinner.Spinner
	Sugar *zap.SugaredLogger
}

// Gcon is gcon app main struct.
type Gcon struct {
	// Now CfgIngo.
	Ci CfgInfo
	// Now TaskInfo.
	Ti TaskInfo

	Store
	Logger
}

var (
	startCfgFile string
	startTaskID  string
	startGcon    *Gcon

	funcs      = make(Funcs)
	cfgInfos   = make(CfgInfos, 0)
	cfgInfosMu = new(sync.Mutex)

	// Regexp pre compile.
	reStore = regexp.MustCompile(`\${([^\$]*?)}`)
	reDate  = regexp.MustCompile(`\${@date\(([^\$]*?)\)}`)
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "gcon",
	Short: "gcon is 'Go Controller'",
	Long: `gcon executes processing according to the config file.
For example, Log func is executed twice when the following config file is used.

  tasks:
    - id: Startup
      normal:
        - func: Log
          args:
            msg: log 1
            stdout: true
        - func: Log
          args:
            msg: log 2

You can specify the configuration file to use with the arguments,
and the execution task.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	RunE: runE,
}

func (p ProcessType) String() string {
	switch p {
	case Normal:
		return "Normal"
	case Error:
		return "Error"
	case Final:
		return "Final"
	}

	return ""
}

// Execute adds all child commands to the root command sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		os.Exit(ExitNG)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	RootCmd.PersistentFlags().StringVar(&startCfgFile, "cfg", "", "config file (default is $HOME/.gcon.toml)")
	RootCmd.PersistentFlags().StringVar(&startTaskID, "task", "Startup", "Start up task id")

}

// initConfig reads in config file and ENV variables if set.
func initConfig() {

	var err error

	viper.SetConfigName(".gcon")
	viper.AddConfigPath("$HOME")
	viper.AddConfigPath(".")
	viper.AutomaticEnv()

	if startCfgFile != "" {
		viper.SetConfigFile(startCfgFile)
	}

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		color.Cyan("Use config file: [%v]", viper.ConfigFileUsed())
		startCfgFile = viper.ConfigFileUsed()
	} else {
		fmt.Fprintln(os.Stderr, err)
		RootCmd.Usage()
		os.Exit(ExitNG)
	}

	startGcon = NewGcon()
	// Store cfg info.
	err = startGcon.setCfg(viper.ConfigFileUsed())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(ExitNG)
	}

	ci, err := startGcon.importCfg(viper.ConfigFileUsed())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(ExitNG)
	}
	z, err := ci.Log.Build()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(ExitNG)
	}
	startGcon.Ci = ci
	startGcon.Logger.Sugar = z.Sugar()

}

func runE(cmd *cobra.Command, args []string) error {

	cmd.SilenceUsage = true

	ti := TaskInfo{
		ID:      startTaskID,
		Path:    startCfgFile,
		ProType: Normal,
	}
	startGcon.setTaskInfo(ti)

	err := startGcon.Engine(ti)
	if err != nil {
		return err
	}

	return nil
}

// Engine execute Process.
func (g *Gcon) Engine(ti TaskInfo) error {

	// Restore previous TaskInfo.
	prevTi := g.Ti
	defer g.setTaskInfo(prevTi)
	// Restore previous CfgInfo.
	prevCi := g.Ci
	defer g.setCfgInfo(prevCi)

	// Get now CfgInfo by TaskInfo.
	ci, err := g.getCfgInfo(ti)
	if err != nil {
		return err
	}

	if ti.Path != "" {
		ti.Path = ci.Path
	}

	// Set now CfgInfo and TaskInfo.
	g.setCfgInfo(ci)
	g.setTaskInfo(ti)

	// Execute final process.
	if g.Ti.ProType == Normal {
		fTi := g.Ti
		fTi.ProType = Final
		defer g.Engine(fTi)
	}

	// Execute Init Task.
	err = g.init()
	if err != nil {
		return err
	}

	// Get process.
	ps, err := g.getProcess()
	if err != nil {
		if g.Ti.ProType == Normal {
			return err
		}
		return nil
	}

	// Execute process.
	for _, f := range ps {
		now := time.Now()
		sfx := color.GreenString("[%v] [%v] [%v] [%v] [%v]", now.Format("2006-01-02 15:04:05.000"), g.Ti.Path, g.Ti.ID, g.Ti.ProType, f.Name)
		fmt.Println(sfx)
		g.Spin.Stop()
		g.Spin.Suffix = " " + sfx
		g.Spin.Start()
		time.Sleep(500 * time.Millisecond)

		// func exists check.
		if _, ok := funcs[f.Name]; !ok {
			return fmt.Errorf("func: [%v] is not defined", f.Name)
		}

		a, err := g.replaceAll(f.Args)
		if err != nil {
			return err
		}
		next, err := funcs[f.Name](g, a.(map[interface{}]interface{}))
		if err != nil && g.Ti.ProType == Normal {
			errTi := g.Ti
			errTi.ProType = Error
			err2 := g.Engine(errTi)
			if err2 != nil {
				return errors.Wrap(err, err2.Error())
			}
			return err
		}
		if next.ID != "" {
			err := g.Engine(next)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// Set key value to Store.
func (g *Gcon) Set(key string, value interface{}, output bool) {

	if output {
		g.Infof("[%v] = [%v]", key, value)
	}
	g.Store[key] = value
}

// Get value by key from Store.
func (g *Gcon) Get(key string) (interface{}, error) {

	if v, ok := g.Store[key]; ok {
		return v, nil
	}
	return nil, fmt.Errorf("Not found key: [%v]", key)
}

func (g *Gcon) setCfg(cfgFile string) error {

	cfgPI, err := file.GetPathInfo(cfgFile)
	if err != nil {
		return err
	}
	g.Set("_CFG_ID", startTaskID, false)
	g.Set("_CFG_DIR", cfgPI.Dir, false)
	g.Set("_CFG_FILE", cfgPI.File, false)
	g.Set("_CFG_NAME", cfgPI.Name, false)
	g.Set("_CFG_FILENAME", cfgPI.FileName, false)

	return nil
}

func (g *Gcon) getProcess() (Process, error) {

	// Get task.
	for _, task := range g.Ci.Tasks {
		if task.ID == g.Ti.ID {
			switch g.Ti.ProType {
			case Normal:
				return task.Normal, nil
			case Error:
				return task.Error, nil
			case Final:
				return task.Final, nil
			default:
				return Process{}, fmt.Errorf("Invalid ProcessType [%v]", g.Ti.ProType)
			}
		}
	}

	return Process{}, fmt.Errorf("Not found process. task id: [%v] task file: [%v]", g.Ti.ID, g.Ti.Path)
}

func (g *Gcon) getCfgInfo(ti TaskInfo) (CfgInfo, error) {

	if ti.Path == g.Ci.Path {
		return g.Ci, nil
	}

	// Search now CfgInfo.
	ci, err := g.searchCfgInfo(ti)
	if err != nil {
		// Import cfg file.
		ci, err := g.importCfg(ti.Path)
		if err != nil {
			return CfgInfo{}, err
		}
		return ci, nil
	}

	return ci, nil
}

func (g *Gcon) importCfg(file string) (CfgInfo, error) {

	cfgInfosMu.Lock()
	defer cfgInfosMu.Unlock()

	if file != "" {
		viper.SetConfigFile(file)
	}

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		color.Cyan("Import config file: [%v]", viper.ConfigFileUsed())
	} else {
		return CfgInfo{}, err
	}

	// Load cfg for replace.
	f, err := ioutil.ReadFile(viper.ConfigFileUsed())
	if err != nil {
		return CfgInfo{}, err
	}
	var a interface{}
	err = yaml.Unmarshal(f, &a)
	if err != nil {
		return CfgInfo{}, err
	}

	cfgMap := make(map[interface{}]interface{})
	cfgMap["tasks"] = a.(map[interface{}]interface{})["tasks"]
	logCfg, ok := a.(map[interface{}]interface{})["log"]
	if ok {
		logCfgRep, err := g.replaceAll(logCfg)
		if err != nil {
			return CfgInfo{}, err
		}
		cfgMap["log"] = logCfgRep
	}

	// Load and store cfg.
	tb, err := yaml.Marshal(cfgMap)
	if err != nil {
		return CfgInfo{}, err
	}

	cfg := Cfg{}
	err = yaml.Unmarshal(tb, &cfg)
	if err != nil {
		return CfgInfo{}, err
	}

	// Set cfgInfo to cfgInfos.
	ci := CfgInfo{
		Path: viper.ConfigFileUsed(),
		Init: false,
		Cfg:  cfg,
	}
	cfgInfos = append(cfgInfos, ci)

	return ci, nil
}

func (g *Gcon) searchCfgInfo(ti TaskInfo) (CfgInfo, error) {

	// Search now CfgInfo.
	for _, task := range g.Ci.Tasks {
		if task.ID == ti.ID {
			return g.Ci, nil
		}
	}

	// Search other CfgInfo.
	cfgInfosMu.Lock()
	defer cfgInfosMu.Unlock()

	for _, ci := range cfgInfos {
		if ci.Path == ti.Path {
			return ci, nil
		}
		for _, task := range ci.Tasks {
			if task.ID == ti.ID {
				return ci, nil
			}
		}
	}

	return CfgInfo{}, fmt.Errorf("Not found task ID: [%v] Path: [%v] ", ti.ID, ti.Path)
}

func (g *Gcon) checkCfg(cfg Cfg) error {

	// Check func.
	check := func(ps Process) error {
		// for _, f := range ps {
		// Check func name.
		// fn, ok := funcs[f.Name]
		// if !ok {
		// 	return fmt.Errorf("func [%v] is not found", f.Name)
		// }
		// Check args.
		// err := fn.s.Parse(f.Args)
		// if err != nil {
		// 	return err
		// }
		// }

		return nil
	}

	// Check all task.
	for _, task := range cfg.Tasks {
		// Check normal process.
		err := check(task.Normal)
		if err != nil {
			return err
		}
		// Check error process.
		err = check(task.Error)
		if err != nil {
			return err
		}
		// Check final process.
		err = check(task.Final)
		if err != nil {
			return err
		}
	}

	return nil
}

func (g *Gcon) init() error {

	if g.Ci.Init {
		return nil
	}

	cfgInfosMu.Lock()

	for i, ci := range cfgInfos {
		if ci.Path == g.Ci.Path {
			cfgInfos[i].Init = true
			g.Ci.Init = true
			cfgInfosMu.Unlock()
			ti := g.Ti
			ti.ID = Init
			return g.Engine(ti)
		}
	}

	cfgInfosMu.Unlock()
	return fmt.Errorf("Not found init task. task file: [%v]", g.Ci.Path)
}

func (g *Gcon) setTaskInfo(ti TaskInfo) {

	pi, _ := file.GetPathInfo(ti.Path)
	g.Set("_TASK_ID", ti.ID, false)
	g.Set("_TASK_DIR", pi.Dir, false)
	g.Set("_TASK_FILE", pi.File, false)
	g.Set("_TASK_NAME", pi.Name, false)
	g.Set("_TASK_FILENAME", pi.FileName, false)

	g.Set("_PROCESS_TYPE", ti.ProType, false)

	g.Ti = ti

}

func (g *Gcon) setCfgInfo(ci CfgInfo) {

	g.Ci = ci
}

func (g *Gcon) replaceAll(iface interface{}) (interface{}, error) {

	var rep func(reflect.Value) (interface{}, error)
	rep = func(rv reflect.Value) (interface{}, error) {

		switch rv.Kind() {
		case reflect.Map:
			g.Debugf("Map: [%v]", rv)
			tm := make(map[interface{}]interface{})
			for _, key := range rv.MapKeys() {
				m, err := rep(rv.MapIndex(key))
				if err != nil {
					return nil, err
				}
				tm[key.Interface()] = m
			}
			return tm, nil
		case reflect.Slice:
			g.Debugf("Slice: [%v]", rv)
			ts := make([]interface{}, 0)
			for i := 0; i < rv.Len(); i++ {
				s, err := rep(rv.Index(i))
				if err != nil {
					return nil, err
				}
				ts = append(ts, s)
			}
			return ts, nil
		case reflect.Int:
			g.Debugf("Int: [%v]", rv.Int())
			return rv.Int(), nil
		case reflect.String:
			g.Debugf("String: [%v]", rv.String())
			rep := g.replace(rv.String())
			return g.typeChange(rep), nil
		case reflect.Bool:
			g.Debugf("Bool: [%v]", rv.Bool())
			return rv.Bool(), nil
		case reflect.Interface:
			g.Debugf("Interface: [%v]", rv.Interface())
			iface, err := rep(rv.Elem())
			if err != nil {
				return nil, err
			}
			return iface, nil
		default:
			return nil, fmt.Errorf("[%v] is not support", rv.Kind())
		}
	}

	return rep(reflect.ValueOf(iface))
}

func (g *Gcon) replace(node string) interface{} {

	retry := false

	// Date replace.
	if reDate.MatchString(node) {
		f := reDate.FindStringSubmatch(node)[1]
		now := time.Now().Format(f)
		color.Red("[%v] -> [%v]", node, now)
		g.Debugf("[%v] -> [%v]", node, now)
		// node = reDate.ReplaceAllString(node, now)
		strings.Replace(node, "${@date("+f+")}", now, 1)
		retry = true
	}

	// Store replace.
	if reStore.MatchString(node) {
		key := reStore.FindStringSubmatch(node)[1]
		rep, err := g.Get(key)
		if err == nil {
			color.Red("[%v] -> [%v]", key, rep)
			g.Debugf("[%v] -> [%v]", node, rep)
			// node = reStore.ReplaceAllString(node, rep.(string))
			strings.Replace(node, "${"+f+"}", rep, 1)
			retry = true
		}
	}

	if retry {
		return g.replace(node)
	}

	return node

}

// ParseArgs make map to struct and check cfg.
func ParseArgs(args Args, ptr interface{}) error {

	t, err := yaml.Marshal(args)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(t, ptr)
	if err != nil {
		return err
	}

	var check func(reflect.Value) error
	check = func(rv reflect.Value) error {

		switch rv.Kind() {
		case reflect.Ptr:
			rv = rv.Elem()
		case reflect.Slice:
			for i := 0; i < rv.Len(); i++ {
				err := check(rv.Index(i))
				if err != nil {
					return err
				}
			}
		}

		rt := rv.Type()

		for i := 0; i < rv.NumField(); i++ {
			fieldV := rv.Field(i)
			fieldT := rt.Field(i)
			req, _ := strconv.ParseBool(fieldT.Tag.Get("require"))

			switch fieldV.Kind() {
			case reflect.Ptr, reflect.Struct:
				err := check(fieldV)
				if err != nil {
					return err
				}
			case reflect.Slice:
				for i := 0; i < fieldV.Len(); i++ {
					err := check(fieldV.Index(i))
					if err != nil {
						return err
					}
				}
			case reflect.Int:
				if req && fieldV.Int() == 0 {
					return fmt.Errorf("[%v.%v] is require. but not set", rt.Name(), fieldT.Name)
				}
			case reflect.String:
				if req && fieldV.String() == "" {
					return fmt.Errorf("[%v.%v] is require. but not set", rt.Name(), fieldT.Name)
				}
			case reflect.Bool:
			default:
				return fmt.Errorf("[%v] is not support", fieldV.Kind())
			}
		}

		return nil
	}

	return check(reflect.ValueOf(ptr))
}

// NewGcon initiallize and return Gcon struct.
func NewGcon() *Gcon {

	g := &Gcon{
		Store: make(Store),
		Logger: Logger{
			Spin: spinner.New(spinner.CharSets[14], 150*time.Millisecond),
		},
	}
	g.Spin.Color("red")
	g.Spin.Stop()
	return g
}

func (g *Gcon) typeChange(src interface{}) interface{} {

	if b, err := strconv.ParseBool(src); err == nil {
		g.Debugf("[%v] -> [%v]", src, b)
		return b
	}

	if n, err := strconv.Atoi(src); err == nil {
		g.Debugf("[%v] -> [%v]", src, n)
		return n
	}

	return src
}

// Infof is wrapper of sugar.Infof.
func (l *Logger) Infof(template string, args ...interface{}) {
	l.Spin.Stop()
	if l.Sugar != nil {
		l.Sugar.Infof(template, args...)
	}
	l.Spin.Start()
}

// Debugf is wrapper of sugar.Debugf.
func (l *Logger) Debugf(template string, args ...interface{}) {
	l.Spin.Stop()
	if l.Sugar != nil {
		l.Sugar.Debugf(template, args...)
	}
	l.Spin.Start()
}
