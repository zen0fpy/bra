// Copyright 2015 Unknwon
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
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/unknwon/com"
	"github.com/unknwon/log"
	"github.com/urfave/cli"

	"github.com/unknwon/bra/internal/bindata"
)

// Init 初始化模板
var Init = cli.Command{
	Name:   "init",
	Usage:  "initialize config template file",
	Action: runInit,
	Flags:  []cli.Flag{},
}

func runInit(ctx *cli.Context) error {

	// 如果.bra.toml配置文件存在，
	// 提示是否覆盖
	if com.IsExist(".bra.toml") {
		fmt.Print("There is a .bra.toml in the work directory, do you want to overwrite?(y/n): ")
		var answer string
		fmt.Scan(&answer)
		if strings.ToLower(answer) != "y" {
			fmt.Println("Existed file is untouched.")
			return nil
		}
	}

	// 当前目录
	wd, err := os.Getwd()
	if err != nil {
		log.Fatal("Fail to get work directory: %v", err)
	}

	// 获取默认模板资源
	data, err := bindata.Asset("templates/default.bra.toml")
	if err != nil {
		log.Fatal("Fail to get asset: %v", err)
	}

	appName := filepath.Base(wd)
	if runtime.GOOS == "windows" {
		appName += ".exe"
	}

	// 替换应用名，生成新的配置文件
	data = bytes.Replace(data, []byte("$APP_NAME"), []byte(appName), -1)
	if err := ioutil.WriteFile(".bra.toml", data, os.ModePerm); err != nil {
		log.Fatal("Fail to generate default .bra.toml: %v", err)
	}
	return nil
}
