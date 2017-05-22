// Copyright © 2017 yukimemi <yukimemi@gmail.com>

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

	"github.com/fatih/color"
	"github.com/gosuri/uilive"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/ulule/deepcopier"
	"github.com/yukimemi/core"
	"github.com/yukimemi/file"
	"go.uber.org/zap"
	"gopkg.in/go-playground/validator.v9"
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

const (
	// Init task id.
	Init = "Init"
	// FuncName is fucntion name.
	FuncName = "_FUNC_NAME"
)

// ProcessType is type of process.
type ProcessType int

// CfgInfo is config file information.
type CfgInfo struct {
	Path string
	Init bool
	*Cfg
}

// Cfg is gcon config file struct.
type Cfg struct {
	Tasks []*Task    `mapstructure:"tasks" yaml:"tasks"`
	Log   zap.Config `mapstructure:"log" yaml:"log"`
}

// CfgInfos is Array of CfgInfo.
type CfgInfos []*CfgInfo

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

// FuncInfo is function information.
type FuncInfo struct {
	ID    int
	Name  string
	Async string
	Done  bool
	Wg    *sync.WaitGroup
	Err   error
}

// ArgsDef is default args.
type ArgsDef struct {
	Async string
}

// Args is func arg.
type Args map[interface{}]interface{}

// Process is array of Func.
type Process []*Func

// Funcs is func map.
type Funcs map[string]func(*Gcon, Args) (*TaskInfo, error)

// Store store string data.
type Store map[string]interface{}

// Logger is log struct.
type Logger struct {
	Sugar *zap.SugaredLogger
}

// Gcon is gcon app main struct.
type Gcon struct {
	// Now CfgIngo.
	Ci CfgInfo
	// Now TaskInfo.
	Ti TaskInfo
	// Now FuncInfo.
	Fi FuncInfo

	Store
	*Logger
}

var (
	startCfgFile string
	startTaskID  string
	startGcon    *Gcon

	funcs      = make(Funcs)
	cfgInfos   = make(CfgInfos, 0)
	cfgInfosMu = new(sync.Mutex)
	funcMu     = new(sync.Mutex)
	funcID     = 0
	chGcon     = make(chan Gcon)
	allDone    = make(chan struct{})
	asyncFuncs = make(map[string]*FuncInfo)
	asyncMu    = new(sync.Mutex)

	validate = validator.New()

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
	startGcon.Ci = *ci
	startGcon.Logger.Sugar = z.Sugar()

	cmdPath, err := core.GetCmdPath(os.Args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(ExitNG)
	}

	// Set cmd info.
	err = startGcon.setCmd(cmdPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(ExitNG)
	}

	// Set pwd.
	pwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(ExitNG)
	}

	startGcon.Set("_PWD", pwd, false)

}

func runE(cmd *cobra.Command, args []string) error {

	cmd.SilenceUsage = true

	ti := TaskInfo{
		ID:      startTaskID,
		Path:    startCfgFile,
		ProType: Normal,
	}
	startGcon.setTaskInfo(ti)

	color.Green("Start: %v", time.Now().Format("2006-01-02 15:04:05.000"))
	fmt.Println("")

	go loopPrint()

	err := startGcon.Engine(ti)
	if err != nil {
		startGcon.Errorf(err.Error())
		return err
	}

	// Wait all func.
	allDone <- struct{}{}
	<-allDone

	fmt.Println("")
	color.Green("End: %v", time.Now().Format("2006-01-02 15:04:05.000"))
	return nil
}

// Engine execute Process.
func (g *Gcon) Engine(ti TaskInfo) error {

	// Restore previous TaskInfo.
	defer g.setTaskInfo(g.Ti)
	// Restore previous CfgInfo.
	defer g.setCfgInfo(g.Ci)

	// Get now CfgInfo by TaskInfo.
	ci, err := g.getCfgInfo(ti)
	if err != nil {
		return err
	}

	if ti.Path != "" {
		ti.Path = ci.Path
	}

	// Set now CfgInfo and TaskInfo.
	g.setCfgInfo(*ci)
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

		g.Fi = FuncInfo{
			ID:   getFuncID(),
			Name: f.Name,
			Done: false,
			Wg:   new(sync.WaitGroup),
		}

		// func exists check.
		if _, ok := funcs[f.Name]; !ok {
			return fmt.Errorf("func: [%v] is not defined", f.Name)
		}

		a, err := g.replaceAll(f.Args)
		if err != nil {
			return err
		}

		execute := func(g *Gcon, f Func, a interface{}) error {
			defer func() {
				time.Sleep(time.Millisecond * 500)
				g.Fi.Done = true
				chGcon <- *g
				g.Infof("--- Func End ID: [%v] Name: [%v] ---", g.Fi.ID, g.Fi.Name)
			}()
			g.Infof("--- Func Start ID: [%v] Name: [%v] ---", g.Fi.ID, g.Fi.Name)
			chGcon <- *g
			g.Set(FuncName, g.Fi.Name, false)
			_g := *g
			next, err := funcs[f.Name](g, a.(map[interface{}]interface{}))
			g.Ci, g.Ti, g.Fi = _g.Ci, _g.Ti, _g.Fi
			g.Set(FuncName, g.Fi.Name, false)
			if err != nil && g.Ti.ProType == Normal {
				errTi := g.Ti
				errTi.ProType = Error
				err2 := g.Engine(errTi)
				if err2 != nil {
					return errors.Wrap(err, err2.Error())
				}
				return err
			}
			if next != nil {
				err := g.Engine(*next)
				if err != nil {
					return err
				}
			}
			return nil
		}

		// Check async.
		ad := &ArgsDef{}
		err = g.ParseArgs(a.(map[interface{}]interface{}), ad)
		if err != nil {
			return err
		}
		if ad.Async != "" {
			// Copy Gcon.
			copyG, err := g.CopyGcon()
			if err != nil {
				return err
			}
			copyG.Fi.Wg.Add(1)
			// Execute asynchronous.
			go func(g *Gcon, f Func, a interface{}) {
				defer g.Fi.Wg.Done()
				g.Fi.Err = execute(g, f, a)
				chGcon <- *g
			}(copyG, *f, a)
		} else {
			// Execute sync.
			err := execute(g, *f, a)
			if err != nil {
				return err
			}
			g.Fi.Err = err
			chGcon <- *g
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
func (g *Gcon) Get(key string) interface{} {

	if v, ok := g.Store[key]; ok {
		return v
	}
	return nil
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

func (g *Gcon) setCmd(cmd string) error {

	cmdPI, err := file.GetPathInfo(cmd)
	if err != nil {
		return err
	}
	g.Set("_CMD_DIR", cmdPI.Dir, false)
	g.Set("_CMD_FILE", cmdPI.File, false)
	g.Set("_CMD_NAME", cmdPI.Name, false)
	g.Set("_CMD_FILENAME", cmdPI.FileName, false)

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

func (g *Gcon) getCfgInfo(ti TaskInfo) (*CfgInfo, error) {

	if ti.Path == g.Ci.Path {
		return &g.Ci, nil
	}

	// Search now CfgInfo.
	ci, err := g.searchCfgInfo(ti)
	if err != nil {
		// Import cfg file.
		ci, err := g.importCfg(ti.Path)
		if err != nil {
			return nil, err
		}
		return ci, nil
	}

	return ci, nil
}

func (g *Gcon) importCfg(file string) (*CfgInfo, error) {

	cfgInfosMu.Lock()
	defer cfgInfosMu.Unlock()

	if file != "" {
		viper.SetConfigFile(file)
	}

	g.Debugf("Use config file (before load): [%v]", viper.ConfigFileUsed())

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		g.Infof("Import config file: [%v]", viper.ConfigFileUsed())
	} else {
		return nil, err
	}

	// Load cfg for replace.
	f, err := ioutil.ReadFile(viper.ConfigFileUsed())
	if err != nil {
		return nil, err
	}
	var a interface{}
	err = yaml.Unmarshal(f, &a)
	if err != nil {
		return nil, err
	}

	cfgMap := make(map[interface{}]interface{})
	cfgMap["tasks"] = a.(map[interface{}]interface{})["tasks"]
	logCfg, ok := a.(map[interface{}]interface{})["log"]
	if ok {
		logCfgRep, err := g.replaceAll(logCfg)
		if err != nil {
			return nil, err
		}
		cfgMap["log"] = logCfgRep
	}

	// Load and store cfg.
	tb, err := yaml.Marshal(cfgMap)
	if err != nil {
		return nil, err
	}

	cfg := Cfg{}
	err = yaml.Unmarshal(tb, &cfg)
	if err != nil {
		return nil, err
	}

	// Set cfgInfo to cfgInfos.
	ci := &CfgInfo{
		Path: viper.ConfigFileUsed(),
		Init: false,
		Cfg:  &cfg,
	}
	cfgInfos = append(cfgInfos, ci)

	return ci, nil
}

func (g *Gcon) searchCfgInfo(ti TaskInfo) (*CfgInfo, error) {

	// Search now CfgInfo.
	for _, task := range g.Ci.Tasks {
		if task.ID == ti.ID {
			return &g.Ci, nil
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

	return nil, fmt.Errorf("Not found task ID: [%v] Path: [%v] ", ti.ID, ti.Path)
}

func (g *Gcon) init() error {

	if g.Ci.Init {
		return nil
	}

	cfgInfosMu.Lock()

	for _, ci := range cfgInfos {
		if ci.Path == g.Ci.Path {
			ci.Init = true
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
			rp := g.replace(rv.String())
			if v, ok := rp.(string); ok {
				return g.typeChange(v), nil
			}
			return rep(reflect.ValueOf(rp))
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
		g.Debugf("[%v] -> [%v]", node, now)
		node = strings.Replace(node, "${@date("+f+")}", now, 1)
		retry = true
	}

	// Store replace.
	if reStore.MatchString(node) {
		key := reStore.FindStringSubmatch(node)[1]
		rep := g.Get(key)
		if rep != nil {
			g.Debugf("[%v] -> [%v]", node, rep)
			rv := reflect.ValueOf(rep)
			switch rv.Kind() {
			case reflect.String, reflect.Int:
				node = strings.Replace(node, "${"+key+"}", rv.String(), 1)
			default:
				return rep
			}
			retry = true
		}
	}

	if retry {
		return g.replace(node)
	}

	return node

}

// ParseArgs make map to struct and check cfg.
func (g *Gcon) ParseArgs(args Args, ptr interface{}) error {

	t, err := yaml.Marshal(args)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(t, ptr)
	if err != nil {
		return err
	}

	// Validate struct.
	err = validate.Struct(ptr)

	if err != nil {
		return err
	}

	return nil
}

// NewGcon initiallize and return Gcon struct.
func NewGcon() *Gcon {

	g := &Gcon{
		Ci:     CfgInfo{},
		Ti:     TaskInfo{},
		Fi:     FuncInfo{},
		Store:  make(Store),
		Logger: new(Logger),
	}
	return g
}

// CopyGcon deep copy Gcon struct.
func (g *Gcon) CopyGcon() (*Gcon, error) {

	dst := NewGcon()
	err := deepcopier.Copy(g).To(dst)

	dst.Store = make(Store)

	for k, v := range g.Store {
		dst.Store[k] = v
	}

	return dst, err

}

func (g *Gcon) typeChange(src string) interface{} {

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
func (g *Gcon) Infof(template string, args ...interface{}) {

	if g.Sugar != nil {
		g.Sugar.Infof(g.makeMsgf(template), args...)
	}
}

// Info is wrapper of sugar.Info.
func (g *Gcon) Info(args ...interface{}) {

	if g.Sugar != nil {
		g.Sugar.Info(g.makeMsg(args))
	}
}

// Debugf is wrapper of sugar.Debugf.
func (g *Gcon) Debugf(template string, args ...interface{}) {

	if g.Sugar != nil {
		g.Sugar.Debugf(g.makeMsgf(template), args...)
	}
}

// Debug is wrapper of sugar.Debug.
func (g *Gcon) Debug(args ...interface{}) {

	if g.Sugar != nil {
		g.Sugar.Debug(g.makeMsg(args))
	}
}

// Errorf is wrapper of sugar.Errorf.
func (g *Gcon) Errorf(template string, args ...interface{}) {

	if g.Sugar != nil {
		g.Sugar.Errorf(g.makeMsgf(template), args...)
	}
}

// Error is wrapper of sugar.Error.
func (g *Gcon) Error(args ...interface{}) {

	if g.Sugar != nil {
		g.Sugar.Error(g.makeMsg(args))
	}
}

func (g *Gcon) makeMsg(args ...interface{}) []interface{} {

	pre := fmt.Sprintf("[%v] [%v] [%v] [%v]:", g.Ti.Path, g.Ti.ID, g.Ti.ProType, g.Fi.Name)
	return append([]interface{}{pre}, args)
}

func (g *Gcon) makeMsgf(template string) string {

	pre := fmt.Sprintf("[%v] [%v] [%v] [%v]: ", g.Ti.Path, g.Ti.ID, g.Ti.ProType, g.Fi.Name)
	return pre + template
}

func loopPrint() {

	writer := uilive.New()
	writer.Start()
	gs := make([]*Gcon, 0)
	spin := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	index := 0
	wait := false

	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()

	for {

		select {
		case gc := <-chGcon:
			exist := false
			for _, g := range gs {
				if g.Fi.ID == gc.Fi.ID {
					*g = gc
					exist = true
					break
				}
			}
			if !exist {
				gs = append(gs, &gc)
			}
		case <-time.After(time.Millisecond * 100):
			index = (index + 1) % len(spin)
			doneCnt := 0
			for _, g := range gs {
				if g.Fi.Done {
					if wait {
						doneCnt++
					}
					fmt.Fprintf(writer, green("%3s   [%s] [%s] [%s] [%s]                    \n"), "✓", g.Ti.Path, g.Ti.ID, g.Ti.ProType, g.Fi.Name)
				} else {
					fmt.Fprintf(writer, yellow("%3s   [%s] [%s] [%s] [%s]                    \n"), spin[index], g.Ti.Path, g.Ti.ID, g.Ti.ProType, g.Fi.Name)
				}
			}
			writer.Flush()
			if wait && doneCnt == len(gs) {
				close(allDone)
				return
			}
		case <-allDone:
			wait = true
		}

	}

}

func getFuncID() int {
	funcMu.Lock()
	defer funcMu.Unlock()

	funcID++
	return funcID
}
