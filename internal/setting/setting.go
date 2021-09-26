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

package setting

import (
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/unknwon/com"
	"github.com/unknwon/log"
)

func init() {
	// 日志前缀, 时间格式
	log.Prefix = "[Bra]"
	log.TimeFormat = "01-02 15:04:05"
}

var (
	WorkDir string
)

var Cfg struct {
	Run struct {
		InitCmds         [][]string       `toml:"init_cmds"`        // init命令
		WatchAll         bool             `toml:"watch_all"`        // 是否监控所有
		WatchDirs        []string         `toml:"watch_dirs"`       // 监控目录列表
		WatchExts        []string         `toml:"watch_exts"`       // 监控文件文件后缀
		IgnoreDirs       []string         `toml:"ignore"`           // 不需要监控目录
		IgnoreFiles      []string         `toml:"ignore_files"`     // 不需要监控文件
		EnvFiles         []string         `toml:"env_files"`        // 环境变量文件
		IgnoreRegexps    []*regexp.Regexp `toml:"-"`                // 不需要监控 支持正则
		FollowSymlinks   bool             `toml:"follow_symlinks"`  // 跟踪链接
		BuildDelay       int              `toml:"build_delay"`      // 延迟构建
		InterruptTimeout int              `toml:"interrupt_timout"` // 中断超时时间
		GracefulKill     bool             `toml:"graceful_kill"`    // 优雅退出
		Cmds             [][]string       `toml:"cmds"`             // 操作命令
	} `toml:"run"`
	Sync struct {
		ListenAddr string `toml:"listen_addr"`                      // 监控地址
		RemoteAddr string `toml:"remote_addr"`                      // 远程监控地址
	} `toml:"sync"`
}

// UnpackPath replaces special path variables and returns full path.
// 替换变量生成完整路径
func UnpackPath(path string) string {
	path = strings.Replace(path, "$WORKDIR", WorkDir, 1)
	// 还要设置GOPATH
	path = strings.Replace(path, "$GOPATH", com.GetGOPATHs()[0], 1)
	return path
}

// IgnoreDir determines whether specified dir must be ignored.
// 要忽略目录
func IgnoreDir(dir string) bool {
	for _, s := range Cfg.Run.IgnoreDirs {
		if strings.Contains(dir, s) {
			return true
		}
	}
	return false
}

// IgnoreFile returns true if file path matches ignore regexp.
func IgnoreFile(file string) bool {
	for i := range Cfg.Run.IgnoreRegexps {
		if Cfg.Run.IgnoreRegexps[i].MatchString(file) {
			return true
		}
	}
	return false
}

func InitSetting() {
	var err error
	// 工作目录
	WorkDir, err = os.Getwd()
	if err != nil {
		log.Fatal("Fail to get work directory: %v", err)
	}

	// 配置文件不存在退出
	// 存在则解析配置
	confPath := path.Join(WorkDir, ".bra.toml")
	if !com.IsFile(confPath) {
		log.Fatal(".bra.toml not found in work directory")
	} else if _, err = toml.DecodeFile(confPath, &Cfg); err != nil {
		log.Fatal("Fail to decode .bra.toml: %v", err)
	}

	// 中断超时时间，如果为0，则设置为15
	if Cfg.Run.InterruptTimeout == 0 {
		Cfg.Run.InterruptTimeout = 15
	}

	// Init default ignore lists.
	// 忽略.git
	Cfg.Run.IgnoreDirs = com.AppendStr(Cfg.Run.IgnoreDirs, ".git")
	Cfg.Run.IgnoreRegexps = make([]*regexp.Regexp, len(Cfg.Run.IgnoreFiles))
	for i, regStr := range Cfg.Run.IgnoreFiles {
		Cfg.Run.IgnoreRegexps[i], err = regexp.Compile(regStr)
		if err != nil {
			log.Fatal("Invalid regexp[%s]: %v", regStr, err)
		}
	}
}
