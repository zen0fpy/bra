// Copyright 2014 Unknwon
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/unknwon/com"
	"github.com/unknwon/log"
	"github.com/urfave/cli"
	"gopkg.in/fsnotify/fsnotify.v1"

	"github.com/unknwon/bra/internal/setting"
)

var (
	lastBuild time.Time
	eventTime = make(map[string]int64)

	runningCmd  *exec.Cmd
	runningLock = &sync.Mutex{}
	shutdown    = make(chan bool)
)

var Run = cli.Command{
	Name:   "run",
	Usage:  "start monitoring and notifying",
	Action: runRun,
	Flags:  []cli.Flag{},
}

// isTmpFile returns true if the event was for temporary files.
// 判断是不是临时文件, 如果带有.tmp后缀文件名就是临时文件
func isTmpFile(name string) bool {
	if strings.HasSuffix(strings.ToLower(name), ".tmp") {
		return true
	}
	return false
}

// hasWatchExt returns true if the file name has watched extension.
// 文件名是否带有要监控后缀扩展
func hasWatchExt(name string) bool {
	for _, ext := range setting.Cfg.Run.WatchExts {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

// runCommand represents a command to run after notified.
// 通知后要执行操作，即通知回调
type runCommand struct {
	Envs []string  // 执行环境变量
	Name string    // 操作名
	Args []string  // 参数

	osEnvAdded bool
}

func (cmd *runCommand) String() string {
	if len(cmd.Envs) > 0 {
		return fmt.Sprintf("%v %s %v", cmd.Envs, cmd.Name, cmd.Args)
	}
	return fmt.Sprintf("%s %v", cmd.Name, cmd.Args)
}

// 解析要执行命令
func parseRunCommand(args []string) *runCommand {
	runCmd := new(runCommand)
	i := 0

	// 例如: GOPATH=/root go run build.go
	// ['GOPATH=/root', 'go' 'run' 'build.go']
	for _, arg := range args {
		// 不存在=， 表示没有用到环境变量
		if !strings.Contains(arg, "=") {
			break
		}
		runCmd.Envs = append(runCmd.Envs, arg)
		i++
	}

	// 添加环境变量
	if len(runCmd.Envs) > 0 {
		runCmd.osEnvAdded = true
		runCmd.Envs = append(runCmd.Envs, os.Environ()...)
	}

	// 过滤环境变量， GOPATH=/root
	// args[i] = go
	// args[i+1:] = ['run', 'build.go']

	runCmd.Name = args[i]
	runCmd.Args = args[i+1:]
	return runCmd
}

func parseRunCommands(cmds [][]string) []*runCommand {
	runCmds := make([]*runCommand, len(cmds))
	for i, args := range cmds {
		runCmds[i] = parseRunCommand(args)
	}
	return runCmds
}

// 从文件中提取环境变量
func envFromFiles() []string {
	envs := make([]string, 0)

	for _, envFile := range setting.Cfg.Run.EnvFiles {
		b, err := ioutil.ReadFile(envFile)
		if err != nil {
			log.Warn("Fail to read env file %q: %v", envFile, err)
			continue
		}

		envLines := strings.Split(string(b), "\n")
		for _, env := range envLines {
			envs = append(envs, strings.TrimPrefix(env, "export "))
		}
	}

	return envs
}

// 通知
func notify(cmds []*runCommand) {
	runningLock.Lock()

	defer func() {
		// 要执行命令置空
		runningCmd = nil
		runningLock.Unlock()
	}()

	for _, cmd := range cmds {
		command := exec.Command(cmd.Name, cmd.Args...)
		// 命令传入环境变量
		command.Env = cmd.Envs

		envFromFiles := envFromFiles()
		if len(envFromFiles) > 0 {
			// 从文件读取环境变量
			command.Env = append(command.Env, envFromFiles...)
			// 添加系统环境
			if !cmd.osEnvAdded {
				command.Env = append(command.Env, os.Environ()...)
			}
		}

		// 设置标准输入和输出, 并执行命令
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		if err := command.Start(); err != nil {
			log.Error("Fail to start command: %v - %v", cmd, err)
			fmt.Print("\x07")
			return
		}

		log.Debug("Running: %s", cmd)
		// 当前操作命令
		runningCmd = command
		done := make(chan error)
		// 启动一个goroutine在后台等待执行结果
		go func() {
			done <- command.Wait()
		}()

		isShutdown := false
		select {
		case err := <-done:
			// 设置关闭标识, 退出
			if isShutdown {
				return
			// 执行报错，也退出
			} else if err != nil {
				log.Warn("Fail to execute command: %v - %v", cmd, err)
				fmt.Print("\x07")
				return
			}
		// 收到关闭信号，退出
		case <-shutdown:
			isShutdown = true
			gracefulKill()
			return
		}
	}
	log.Info("Notify operations are done!")
}

func gracefulKill() {
	// Directly kill the process on Windows or under request.
	// windows系统, 强杀
	if runtime.GOOS == "windows" || !setting.Cfg.Run.GracefulKill {
		runningCmd.Process.Kill()
		return
	}

	// Given process a chance to exit itself.
	// 发送一个中断信息
	runningCmd.Process.Signal(os.Interrupt)

	// Wait for timeout, and force kill after that.
	for i := 0; i < setting.Cfg.Run.InterruptTimeout; i++ {
		time.Sleep(1 * time.Second)

		if runningCmd.ProcessState == nil || runningCmd.ProcessState.Exited() {
			return
		}
	}
	log.Info("Fail to graceful kill, force killing...")
	runningCmd.Process.Kill()
}

func catchSignals() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM)
	<-sigs

	if runningCmd != nil {
		shutdown <- true
	}
	os.Exit(0)
}

func runRun(ctx *cli.Context) error {
	// 初始化中断超时时间, 忽略目录
	setup(ctx)

	// 获取到退出信息，执行退出操作
	go catchSignals()
	// 后台执行命令
	go notify(parseRunCommands(setting.Cfg.Run.InitCmds))

	// 监控路径列表
	watchPathes := append([]string{setting.WorkDir}, setting.Cfg.Run.WatchDirs...)

	// 监控所有文件, 就去获取子目录
	if setting.Cfg.Run.WatchAll {
		// 监控10个子目录
		subdirs := make([]string, 0, 10)
		// watchPaths[0]第一个是当前工作目录
		for _, dir := range watchPathes[1:] {
			var dirs []string
			var err error

			// 软连接 ，就是去获取真实路径
			if setting.Cfg.Run.FollowSymlinks {
				dirs, err = com.LgetAllSubDirs(setting.UnpackPath(dir))
			} else {
				dirs, err = com.GetAllSubDirs(setting.UnpackPath(dir))
			}

			if err != nil {
				log.Fatal("Fail to get sub-directories: %v", err)
			}

			for i := range dirs {
				if !setting.IgnoreDir(dirs[i]) {
					subdirs = append(subdirs, path.Join(dir, dirs[i]))
				}
			}
		}
		watchPathes = append(watchPathes, subdirs...)
	}

	// 创建文件监控器
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal("Fail to create new watcher: %v", err)
	}
	defer watcher.Close()

	// 启动一个goroutine
	go func() {
		runCmds := parseRunCommands(setting.Cfg.Run.Cmds)

		for {
			select {
			// 文件事件
			case e := <-watcher.Events:
				needsNotify := true

				// 文件是临时文件, 不需要监控后缀文件， 或者忽略的文件
				if isTmpFile(e.Name) || !hasWatchExt(e.Name) || setting.IgnoreFile(e.Name) {
					continue
				}

				// Prevent duplicated builds.
				if lastBuild.Add(time.Duration(setting.Cfg.Run.BuildDelay) * time.Millisecond).
					After(time.Now()) {
					continue
				}
				lastBuild = time.Now()

				showName := e.String()
				if !log.NonColor {
					showName = strings.Replace(showName, setting.WorkDir, "\033[47;30m$WORKDIR\033[0m", 1)
				}

				// 不是删除，或者重命名
				if e.Op&fsnotify.Remove != fsnotify.Remove && e.Op&fsnotify.Rename != fsnotify.Rename {
					// 文件修改时间
					mt, err := com.FileMTime(e.Name)
					if err != nil {
						log.Error("Fail to get file modify time: %v", err)
						continue
					}
					// 修改时间没有变化
					if eventTime[e.Name] == mt {
						log.Debug("Skipped %s", showName)
						needsNotify = false
					}
					eventTime[e.Name] = mt
				}

				if needsNotify {
					log.Info(showName)
					if runningCmd != nil && runningCmd.Process != nil {
						if runningCmd.Args[0] == "sudo" && runtime.GOOS == "linux" {
							// 给父进程发送一个TERM信号，试图杀死它和它的子进程
							rootCmd := exec.Command("sudo", "kill", "-TERM", com.ToStr(runningCmd.Process.Pid))
							rootCmd.Stdout = os.Stdout
							rootCmd.Stderr = os.Stderr
							if err := rootCmd.Run(); err != nil {
								log.Error("Fail to start rootCmd %s", err.Error())
								fmt.Print("\x07")
							}
						} else {
							shutdown <- true
						}
					}
					go notify(runCmds)
				}
			}
		}
	}()

	log.Info("Following directories are monitored:")
	for i, p := range watchPathes {
		if err = watcher.Add(setting.UnpackPath(p)); err != nil {
			log.Fatal("Fail to watch directory(%s): %v", p, err)
		}
		if i > 0 && !log.NonColor {
			p = strings.Replace(p, setting.WorkDir, "\033[47;30m$WORKDIR\033[0m", 1)
			p = strings.Replace(p, "$WORKDIR", "\033[47;30m$WORKDIR\033[0m", 1)
		}
		fmt.Printf("-> %s\n", p)
	}
	select {}
	return nil
}
