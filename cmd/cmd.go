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
	"github.com/unknwon/log"
	"github.com/urfave/cli"

	"github.com/unknwon/bra/internal/setting"
)

var AppVer string
// 配置初始化
func setup(ctx *cli.Context) {
	log.Info("App Version: %s", AppVer)
	setting.InitSetting()
}
