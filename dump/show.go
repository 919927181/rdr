package dump

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"time"

	"github.com/919927181/rdr/decoder"
	"github.com/919927181/rdr/static"
	assetfs "github.com/elazarl/go-bindata-assetfs"
	"github.com/julienschmidt/httprouter"
	"github.com/urfave/cli"
)

/**
 * @file about: 通过网页显示rdb file的统计信息
 */

var counters = NewSafeMap()

func listPathFiles(pathname string) []string {
	var filenames []string
	fi, err := os.Lstat(pathname) // For read access.
	if err != nil {
		return filenames
	}
	if fi.IsDir() {
		files, err := ioutil.ReadDir(pathname)
		if err != nil {
			log.Fatal(err)
		}
		for _, f := range files {
			name := path.Join(pathname, f.Name())
			filenames = append(filenames, name)
		}
	} else {
		filenames = append(filenames, pathname)
	}
	return filenames
}

// Show parse rdbfile(s) and show statistical information by html
func Show(c *cli.Context) {
	if c.NArg() < 1 {
		fmt.Fprintln(c.App.ErrWriter, "show requires at least 1 argument")
		cli.ShowCommandHelp(c, "show")
		return
	}

	// top N bigkey (按内存)
	topN := c.Int("num")
	// 将带单位的字符串转换为字节数, GetLargestEntries 过滤掉小于阈值sizeFilter的key，传0表示不过滤
	sizeFilter, err := ParseUnitToBytes(c.String("size"))
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	// Separators
	separators := c.String("separators")

	// Store all prefixes
	storeAllPrefixes := c.Bool("store-all-prefixes")

	// Top prefixes
	topPrefixN := c.Int("top-prefix")

	// Prefix shrink
	prefixShrink := c.Int("prefix-shrink")

	// Prefix capacity
	prefixMaxCapacity := c.Int("prefix-max-capacity")

	// Counter config
	counterConfig := &CounterConfig{
		TopBigKeyNum:            topN,
		Separators:              separators,
		StoreAllPrefixes:        storeAllPrefixes,
		TopPrefixNum:            topPrefixN,
		PrefixPreShrinkNum:      prefixShrink,
		PrefixContainerMaxCapacity: prefixMaxCapacity,
		Aux_Ctime:        0 ,
	}

	// parse rdbfile
	fmt.Fprintln(c.App.Writer, "start ...")

	instances := []string{}
	InitHTMLTmpl()
	go func() {
		for {
			for _, pathname := range c.Args() {
				for _, v := range listPathFiles(pathname) {
					filename := filepath.Base(v)

					if !counters.Check(filename) {
						decoder := decoder.NewDecoder()
						startMilliseconds := time.Now().UnixMilli()
						fmt.Fprintf(c.App.Writer, "start to parse %v \n", filename)
						go Decode(c, decoder, v)
                        // rdb 解析，会先读基础信息（如rdb版本，aux元属性），然后再去解析key，它是异步解析的，因此这里先等下，等到读出rdb创建时间aux_ctime，再去执行count
						for {
							if  decoder.GetTimestamp() !=0 {
								break
							}
							time.Sleep(200 * time.Millisecond) //暂停200毫秒
						}
						counterConfig.Aux_Ctime=decoder.GetTimestamp()
						counter := NewCounter(counterConfig)
						counter.Count(decoder.Entries)
						counters.Set(filename, counter)
						endMilliseconds := time.Now().UnixMilli()
						fmt.Fprintf(c.App.Writer, "parse %v done, time use %d ms.\n", filename, (endMilliseconds - startMilliseconds))
						instances = append(instances, filename)
						// init html template
						// init common data in template
						tplCommonData["Instances"] = instances
						tplCommonData["TopN"] = strconv.Itoa(topN)
						tplCommonData["sizeFilter"] = strconv.FormatInt(sizeFilter, 10)
					}
				}

			}
			time.Sleep(5 * time.Second)
		}
	}()

	// start http server
	staticFS := assetfs.AssetFS{
		Asset:     static.Asset,
		AssetDir:  static.AssetDir,
		AssetInfo: static.AssetInfo,
	}
	router := httprouter.New()
	router.ServeFiles("/static/*filepath", &staticFS)
	router.GET("/", index)
	router.GET("/instance/:path", rdbReveal)
	fmt.Fprintln(c.App.Writer, "when parsing finished, please access http://{$IP}:"+c.String("port"))
	listenErr := http.ListenAndServe(":"+c.String("port"), router)
	if listenErr != nil {
		fmt.Fprintf(c.App.ErrWriter, "Listen port err: %v\n", listenErr)
	}
}
