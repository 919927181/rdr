package dump

import (
	"bufio"
	"container/heap"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/919927181/rdb"
	"github.com/919927181/rdr/decoder"
	"github.com/urfave/cli"
)

/**
 * @file about: 将统计结果输出到STDOUT 或 File
 */

// Dump rdb file statistical information, 此方法不再用了，因为有了输出到STDOUT 或 file
func Dump(path string) (map[string]interface{}, error) {
	topN := 300     // top N bigkey (按内存),最大500
	sizeFilter := 0 // GetLargestEntries 过滤掉小于阈值的key，传0表示不过滤

	var data map[string]interface{}
	decoder := decoder.NewDecoder()
	go func() {
		f, err := os.Open(path)
		defer close(decoder.Entries)
		if err != nil {
			fmt.Printf("open rdb file err: %v\n", err)
			return
		}
		err = rdb.Decode(f, decoder)
		if err != nil {
			fmt.Printf("decode rdb file err: %v\n", err)
			return
		}
	}()
	cnt := NewCounter(NewCounterConfig())
	cnt.Count(decoder.Entries)
	filename := filepath.Base(path)
	data = GetData(filename, cnt, topN, int64(sizeFilter))
	return data, nil
}

// ToCliWriter dump rdb file statistical information to STDOUT.
func ToCliWriter(cli *cli.Context) {
	if cli.NArg() < 1 {
		fmt.Fprintln(cli.App.ErrWriter, " requires at least 1 argument")
		return
	}
	topN := 100     // top N bigkey (按内存),最大500
	sizeFilter := 0 // GetLargestEntries 过滤掉小于阈值的key，传0表示不过滤

	// parse rdb file
	fmt.Fprintln(cli.App.Writer, "[")
	nArgs := cli.NArg()
	for i := 0; i < nArgs; i++ {
		file := cli.Args().Get(i)
		rdbDecoder := decoder.NewDecoder()
		go Decode(cli, rdbDecoder, file)
		cnt := NewCounter(NewCounterConfig())
		cnt.Count(rdbDecoder.Entries)
		filename := filepath.Base(file)
		data := GetData(filename, cnt, topN, int64(sizeFilter))
		data["MemoryUse"] = rdbDecoder.GetUsedMem()
		data["CTime"] = rdbDecoder.GetTimestamp()
		jsonBytes, _ := json.MarshalIndent(data, "", "    ")
		fmt.Fprint(cli.App.Writer, string(jsonBytes))
		if i == nArgs-1 {
			fmt.Fprintln(cli.App.Writer)
		} else {
			fmt.Fprintln(cli.App.Writer, ",")
		}
	}
	fmt.Fprintln(cli.App.Writer, "]")
}

// ToCliWriter dump rdb file statistical information to file.
func ToCliWriterToFile(cli *cli.Context) {
	if cli.NArg() < 1 {
		fmt.Fprintln(cli.App.ErrWriter, " requires at least 1 argument")
		return
	}

	// top N bigkey (按内存)
	topN := cli.Int("num")
	// 将带单位的字符串转换为字节数, GetLargestEntries 过滤掉小于阈值sizeFilter的key，传0表示不过滤
	sizeFilter, err := ParseUnitToBytes(cli.String("size"))
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Separators
	separators := cli.String("separators")

	// Store all prefixes
	storeAllPrefixes := cli.Bool("store-all-prefixes")

	// Top prefixes
	topPrefixN := cli.Int("top-prefix")

	// Prefix shrink
	prefixShrink := cli.Int("prefix-shrink")

	// Prefix capacity
	prefixMaxCapacity := cli.Int("prefix-max-capacity")

	// Counter config
	counterConfig := &CounterConfig{
		TopBigKeyNum:            topN,
		StoreAllPrefixes:        storeAllPrefixes,
		Separators:              separators,
		TopPrefixNum:            topPrefixN,
		PrefixPreShrinkNum:      prefixShrink,
		PrefixContainerMaxCapacity: prefixMaxCapacity,
	}

	cst := time.FixedZone("CST", 8*60*60)
	currentTime := time.Now().In(cst)
	resultFileName := fmt.Sprintf("rdb-report-%s.json", currentTime.Format("20060102-150405"))

	// 创建或打开文件，追加模式; os.O_APPEND|os.O_CREATE|os.O_WRONLY表示以追加模式打开文件，如果文件不存在则创建，并且只允许写入0644
	file, err := os.OpenFile(resultFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("create report file %s failed!", resultFileName)
		return
	}
	defer file.Close()

	//使用 bufio.Writer 可减少系统调用，提升性能,适合大量数据
	writer := bufio.NewWriter(file)
	defer writer.Flush() // WriteString 后内容暂存在缓冲区，必须调用 Flush() 才会真正写入磁盘文件

	// parse rdb file
	fmt.Fprintln(cli.App.Writer, "start parsing...")
	startMilliseconds := time.Now().UnixMilli()
	// 写入内容，换行需要手动加\n
	writer.WriteString("[\n")

	// parse rdb file
	nArgs := cli.NArg()
	for i := 0; i < nArgs; i++ {
		rdbFile := cli.Args().Get(i)
		rdbDecoder := decoder.NewDecoder()
		go Decode(cli, rdbDecoder, rdbFile)
		cnt := NewCounter(counterConfig)
		cnt.Count(rdbDecoder.Entries)
		filename := filepath.Base(rdbFile)
		data := GetData(filename, cnt, topN, sizeFilter)
		data["MemoryUse"] = rdbDecoder.GetUsedMem()
		data["CTime"] = rdbDecoder.GetTimestamp()
		jsonBytes, _ := json.MarshalIndent(data, "", "    ")
		writer.WriteString(string(jsonBytes))
		if i == nArgs-1 {
			writer.WriteString("\n")
		} else {
			writer.WriteString(",\n")
		}
	}
	writer.WriteString("]\n")
	endMilliseconds := time.Now().UnixMilli()
	fmt.Printf("parsing finished, result write to file ./%s.\n", resultFileName)
	fmt.Printf("time use %d ms.\n", (endMilliseconds - startMilliseconds))
}

// Decode ...
func Decode(c *cli.Context, decoder *decoder.Decoder, filepath string) {
	f, err := os.Open(filepath)
	if err != nil {
		fmt.Fprintf(c.App.ErrWriter, "open rdb file err: %v\n", err)
		close(decoder.Entries)
		return
	}
	err = rdb.Decode(f, decoder)
	if err != nil {
		fmt.Fprintf(c.App.ErrWriter, "decode rdb file err: %v\n", err)
		close(decoder.Entries)
		return
	}
}

func GetData(filename string, cnt *Counter, topN int, sizeFilter int64) map[string]interface{} {
	data := make(map[string]interface{})
	data["CurrentInstance"] = filename
	data["LargestKeys"] = cnt.GetLargestEntries(topN, sizeFilter) //top N bigkey (按内存)，sizeFilter过滤小于阈值的key

	largestKeyPrefixesByType := map[string][]*PrefixEntry{}
	for _, entry := range cnt.GetLargestKeyPrefixes() {
		// if mem usage is less than 1M, and the list is long enough, then it's unnecessary to add it.
		if entry.Bytes < 1000*1000 && len(largestKeyPrefixesByType[entry.Type]) > 50 {
			continue
		}
		largestKeyPrefixesByType[entry.Type] = append(largestKeyPrefixesByType[entry.Type], entry)
	}
	data["LargestKeyPrefixes"] = largestKeyPrefixesByType

	data["TypeBytes"] = cnt.typeBytes
	data["TypeNum"] = cnt.typeNum
	totalNum := uint64(0)
	for _, v := range cnt.typeNum {
		totalNum += v
	}
	totalBytes := uint64(0)
	for _, v := range cnt.typeBytes {
		totalBytes += v
	}
	data["TotalNum"] = totalNum
	data["TotalBytes"] = totalBytes

	lenLevelCount := map[string][]*PrefixEntry{}
	for _, entry := range cnt.GetLenLevelCount() {
		lenLevelCount[entry.Type] = append(lenLevelCount[entry.Type], entry)
	}
	data["LenLevelCount"] = lenLevelCount

	var slotBytesHeap slotHeap
	var slotSizeHeap slotHeap
	// 合并循环：只遍历一次 map 结构，分别取出 Bytes 和 Num 数据压入各自堆中
	for slot, stat := range cnt.slotStats {
		heap.Push(&slotBytesHeap, &SlotEntry{
			Slot: slot, Size: stat.Bytes, // 取 Bytes
		})
		heap.Push(&slotSizeHeap, &SlotEntry{
			Slot: slot, Size: stat.Num,   // 取 Num
		})
	}

	slotBytes := make(slotHeap, 0, topN)
	slotNums := make(slotHeap, 0, topN)

	for i := 0; i < topN; i++ {
		continueFlag := false
		if slotBytesHeap.Len() > 0 {
			continueFlag = true
			slotBytes = append(slotBytes, heap.Pop(&slotBytesHeap).(*SlotEntry))
		}
		if slotSizeHeap.Len() > 0 {
			continueFlag = true
			slotNums = append(slotNums, heap.Pop(&slotSizeHeap).(*SlotEntry))
		}

		if !continueFlag {
			break
		}
	}

	data["SlotBytes"] = slotBytes
	data["SlotNums"] = slotNums

	data["ExpireStatBytes"] = cnt.expireStatBytes
	data["ExpireStatNum"] = cnt.expireStatNum

	data["ByteLevelNum"] = cnt.byteLevelNum
	data["ByteLevelBytes"] = cnt.byteLevelBytes

	return data
}
