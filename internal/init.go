// Copyright 2020 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal

import (
	"log"

	"github.com/prometheus-community/json_exporter/config"
	"github.com/urfave/cli"
)

func Init(c *cli.Context) {
	args := c.Args()

	if len(args) < 1 {
		cli.ShowAppHelp(c) //nolint:errcheck
		log.Fatalf("Not enought arguments")
	}

	var (
		configPath = args[0]
	)

	_, err := config.LoadConfig(configPath)

	if err != nil {
		log.Fatal("Failed to load config")
	}
}