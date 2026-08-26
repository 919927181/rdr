// Copyright 2017 XUEQIU.COM
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	// _ "net/http/pprof"

	"os"

	"github.com/919927181/rdr/dump"
	"github.com/urfave/cli"
)

//go:generate go-bindata -prefix "static/" -o=static/static.go -pkg=static -ignore static.go static/...
//go:generate go-bindata -prefix "views/" -o=views/views.go -pkg=views -ignore views.go views/...

func main() {
	// 调试，使用 pprof 进行性能分析，放在这里
	// runtime.GC()  //调用 runtime.GC() 可以强制进行一次垃圾回收，让profile更准确
    // go func() {
    //     log.Println(http.ListenAndServe("127.0.0.1:6060", nil))
    // }()

	app := cli.NewApp()
	app.Name = "rdr"
	app.Usage = "a tool to parse redis rdb file"
	app.Version = "v1.1.9"
	app.Writer = os.Stdout
	app.ErrWriter = os.Stderr
	app.Commands = []cli.Command{
		{
			Name:      "dump",
			Usage:     "dump statistical report of rdb to STDOUT",
			ArgsUsage: "FILE1 [FILE2] [FILE3]...",
			Action:    dump.ToCliWriter,
		},
		{
			Name:      "dump2file",
			Usage:     "dump statistical report of rdb to file(./rdb-report-xxx.json)",
			ArgsUsage: "FILE1 [FILE2] [FILE3]...",
			Flags: []cli.Flag{
				cli.IntFlag{
					Name:  "num, n",
					Value: 300,
					Usage: " top N big keys(By Count Byte), max is 500",
				},
				cli.StringFlag{
					Name:  "size, s",
					Value: "0kb",
					Usage: " when GetLargestEntries, keys smaller than the threshold are filtered. supported units is B/KB/MB/GB, and can be lowercase",
				},
				cli.StringFlag{
					Name:  "separators, sep",
					Value: ":;,_- ",
					Usage: " separators for key prefixes, default is :;,_- ",
				},
				cli.BoolFlag{
					Name:  "store-all-prefixes, spa",
					Usage: " store all key prefixes, default is false",
				},
				cli.IntFlag{
					Name:  "top-prefix, pn",
					Value: 500,
					Usage: " top N key prefixes, default is 500",
				},
				cli.IntFlag{
					Name:  "prefix-shrink, psn",
					Value: 5000,
					Usage: " prefix shrink num, default is 5000",
				},
				cli.IntFlag{
					Name:  "prefix-max-capacity, pmn",
					Value: 50000,
					Usage: " prefix container max capacity, default is 50000",
				},
			},
			Action: dump.ToCliWriterToFile,
		},
		{
			Name:      "show",
			Usage:     "show statistical report of rdb by webpage",
			ArgsUsage: "DIR1 [DIR2] [DIR3] or FILE1 [FILE2] [FILE3]...",
			Flags: []cli.Flag{
				cli.UintFlag{
					Name:  "port, p",
					Value: 8080,
					Usage: "Port for rdr to listen",
				},
				cli.IntFlag{
					Name:  "num, n",
					Value: 300,
					Usage: " top N big keys, max is 500",
				},
				cli.StringFlag{
					Name:  "size, s",
					Value: "0kb",
					Usage: " when GetLargestEntries, keys smaller than the threshold are filtered. supported units is B/KB/MB/GB, and can be lowercase",
				},
				cli.StringFlag{
					Name:  "separators, sep",
					Value: ":;,_- ",
					Usage: " separators for key prefixes, default is :;,_- ",
				},
				cli.BoolFlag{
					Name:  "store-all-prefixes, spa",
					Usage: " store all key prefixes, default is false",
				},
				cli.IntFlag{
					Name:  "top-prefix, pn",
					Value: 500,
					Usage: " top N key prefixes, default is 500",
				},
				cli.IntFlag{
					Name:  "prefix-shrink, psn",
					Value: 5000,
					Usage: " prefix shrink num, default is 5000",
				},
				cli.IntFlag{
					Name:  "prefix-max-capacity, pmn",
					Value: 50000,
					Usage: " prefix container max capacity, default is 50000",
				},
			},
			Action: dump.Show,
		},
		{
			Name:      "keys",
			Usage:     "write all keys of rdb to file(./rdb-all-keys-xxx.txt)",
			ArgsUsage: "FILE1 [FILE2] [FILE3]...",
			Action:    dump.Export_All_Keys,
		},
	}
	app.CommandNotFound = func(c *cli.Context, command string) {
		fmt.Fprintf(c.App.ErrWriter, "command %q can not be found.\n", command)
		cli.ShowAppHelp(c)
	}
	app.Run(os.Args)
}
