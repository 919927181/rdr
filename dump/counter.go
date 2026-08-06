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

package dump

import (
	"container/heap"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/919927181/rdr/decoder"
)

// v1.1.4 add
const (

	DefaultTopBigKeyNum  = 500
	DefaultSeparators = ":;,_-@# "
	DefaultStoreAllPrefixes        = false
	DefaultTopPrefixNum            = 500
	DefaultPrefixPreShrinkNum      = 5000
	DefaultPrefixContainerMaxCapacity = 50000

	// 过期剩余时间分析
	ExpireStat0Str          = "不过期"
	ExpireStat1Str          = "已过期"
	EExpireStat0_1hStr       = "0~1h"
	ExpireStat1_3hStr       = "1~3h"
	ExpireStat3_12hStr      = "3~12h"
	ExpireStat12_24hStr     = "12~24h"
	ExpireStat1_3dStr       = "1~3d"
	ExpireStat3_7dStr       = "3~7d"
	ExpireStat7dStr         = ">7d"
	// rdb创建时间
	EDefaultAuxCtime = 0

	//按key的元素数量，进行区间分析，注意，要和NewCounter中的LengthLevel一致
	LengthLevelStr    = "空"
	LengthLevelStr0   = "0~100"
	LengthLevelStr1   = "100~1k"
	LengthLevelStr2   = "1k~5k"
	LengthLevelStr3   = "5k~1w"
	LengthLevelStr4   = "1w~10w"
	LengthLevelStr5   = ">10w"

	//按key的大小，进行区间分析
	ByteLevelStr1k = "0~1k"
	ByteLevelStr10k = "1k~10k"
	ByteLevelStr100k = "10k~100k"
	ByteLevelStr1M = "100k~1M"
	ByteLevelStr10M = "1M~10M"
	ByteLevelStr100M = "10M~100M"
	ByteLevelStrAbove100M = ">100M"

)
// v1.1.4 add
type CounterConfig struct {

	// Bigkey数量阈值，默认 500
	TopBigKeyNum int

	// key前缀分隔符，默认 ":;,_- "
	Separators string
	// 是否存储所有前缀，仅当你的主机内存足够时开启，默认关闭
	// false时，对前缀的存储量进行动态收缩, 前缀个数越多，创建的对象也就越多，从而耗内存就越多,就会因内存不足执行不完, 详见 https://github.com/919927181/rdr/issues/1
	StoreAllPrefixes bool
	// 前缀数量阈值，默认 500
	TopPrefixNum int
	// 前缀容器的最大容量（容量上限），可以理解为最大水位线，到达最大水位线，就要去排出一部分水，容量上限不要太大（如果你的主机内存比较小，就设置小点）
	PrefixContainerMaxCapacity int
	// 前缀容器的预缩容数量，可以理解为正常水位线。建议缩容目标 PrefixPreShrinkNum 设为容量上限的 10%\30%\50%，避免频繁触发
	PrefixPreShrinkNum int

	// rdb的创建时间，用于过期剩余时间分析
	Aux_Ctime int64
}

// 默认配置, v1.1.4 add
func NewCounterConfig() *CounterConfig {
	return &CounterConfig{
		TopBigKeyNum:                DefaultTopBigKeyNum,
		Separators:                  DefaultSeparators,
		StoreAllPrefixes:            DefaultStoreAllPrefixes,
		TopPrefixNum:                DefaultTopPrefixNum,
		PrefixContainerMaxCapacity:  DefaultPrefixContainerMaxCapacity,
		PrefixPreShrinkNum:          DefaultPrefixPreShrinkNum,
		Aux_Ctime:                   EDefaultAuxCtime,
	}
}

// NewCounter 初始化 NewCounter return a pointer of Counter
func NewCounter(config *CounterConfig) *Counter {
	if config == nil {
		config = NewCounterConfig()
	}
	h := &entryHeap{}
	heap.Init(h)
	p := &prefixHeap{}
	heap.Init(p)
	return &Counter{
		largestEntries:     h,
		largestKeyPrefixes: p,
		lengthLevel0:       100,
		lengthLevel1:       1000,
		lengthLevel2:       5000,
		lengthLevel3:       10000,
		lengthLevel4:       100000,
		lengthLevelBytes:   map[typeKey]uint64{},
		lengthLevelNum:     map[typeKey]uint64{},
		keyPrefixStats:     make(map[typeKey]*prefixStat, 1024), // 初始化 prefixStats, 注意是指针 map
		typeBytes:          map[string]uint64{},
		typeNum:            map[string]uint64{},
		slotStats:          make(map[int]SlotStat, 1024),
		expireStatBytes:    map[string]uint64{ ExpireStat0Str:0, ExpireStat1Str:0, EExpireStat0_1hStr:0, ExpireStat1_3hStr:0, ExpireStat3_12hStr:0,
												ExpireStat12_24hStr:0, ExpireStat1_3dStr:0, ExpireStat3_7dStr:0, ExpireStat7dStr:0},
		expireStatNum:      map[string]uint64{ ExpireStat0Str:0, ExpireStat1Str:0, EExpireStat0_1hStr:0, ExpireStat1_3hStr:0, ExpireStat3_12hStr:0,
												ExpireStat12_24hStr:0, ExpireStat1_3dStr:0, ExpireStat3_7dStr:0, ExpireStat7dStr:0},
		expireStatOrder:    []string{ExpireStat0Str, ExpireStat1Str, EExpireStat0_1hStr, ExpireStat1_3hStr, ExpireStat3_12hStr,
										ExpireStat12_24hStr, ExpireStat1_3dStr, ExpireStat3_7dStr, ExpireStat7dStr},  // 定义顺序
		byteLevelNum:       map[string]uint64{ByteLevelStr1k:0, ByteLevelStr10k:0, ByteLevelStr100k:0, ByteLevelStr1M:0, ByteLevelStr10M:0, ByteLevelStr100M:0, ByteLevelStrAbove100M:0},
		byteLevelBytes:     map[string]uint64{ByteLevelStr1k:0, ByteLevelStr10k:0, ByteLevelStr100k:0, ByteLevelStr1M:0, ByteLevelStr10M:0, ByteLevelStr100M:0, ByteLevelStrAbove100M:0},
		byteLevelOrder:	    []string{ByteLevelStr1k, ByteLevelStr10k, ByteLevelStr100k, ByteLevelStr1M, ByteLevelStr10M, ByteLevelStr100M, ByteLevelStrAbove100M},
		config:             config,
	}
}

// Counter 统计器 Counter for redis memory usage
type Counter struct {
	largestEntries     *entryHeap
	largestKeyPrefixes *prefixHeap  //保留，用于快速获取topN from heap
	lengthLevel0       uint64
	lengthLevel1       uint64
	lengthLevel2       uint64
	lengthLevel3       uint64
	lengthLevel4       uint64
	lengthLevelBytes   map[typeKey]uint64
	lengthLevelNum     map[typeKey]uint64
	keyPrefixStats     map[typeKey]*prefixStat  //合并后的前缀统计表，代替之前的三个 Map
	typeBytes          map[string]uint64
	typeNum            map[string]uint64
	slotStats          map[int]SlotStat
	expireStatBytes    map[string]uint64
	expireStatNum      map[string]uint64
	expireStatOrder    []string  //因为map是无序集合，想要有序需要自己实现，这里定义顺序
	byteLevelNum       map[string]uint64
	byteLevelBytes     map[string]uint64
	byteLevelOrder     []string
	config             *CounterConfig
}

type SlotStat struct {
    Num   uint64
    Bytes uint64
}

// Count by various dimensions，show.go NewCounter 后，调用此方法，遍历decoder的entry, <-chan表示一个只能接收数据的单向通道
func (c *Counter) Count(in <-chan *decoder.Entry) {
	for e := range in {
		c.count(e)   //调下面的count方法（串行分析）
	}
	// get largest prefixes
	c.calcuLargestKeyPrefix(c.config.TopPrefixNum)
}

// 传入一个entry，执行各指标的count方法
func (c *Counter) count(e *decoder.Entry) {
	c.countLargestEntries(e, 500)
	c.countByType(e)
	c.countByLength(e)
	c.countByKeyPrefix(e)  //前缀计数
	c.countBySlot(e)
	// c.countByDb(e) // 该方法由caiqing0204添加，未使用
	c.countByExpire(e)
	c.countByByte(e)

}

// 该方法由caiqing0204添加，没有看到哪儿用到，这里会导致前缀所属db不正确
// func (c *Counter) countByDb(e *decoder.Entry) {
// 	key := typeKey{
// 		Type: e.Type,
// 		Key:  e.Key,
// 	}
// 	c.keyPrefixDb[key] = strconv.Itoa(e.Db)
// }

// GetLargestEntries from heap, num max is 500. 过滤掉小于阈值的key
func (c *Counter) GetLargestEntries(num int, sizeFilter int64) []*decoder.Entry {
	res := []*decoder.Entry{}

	// get a copy of c.largestEntries
	for i := 0; i < c.largestEntries.Len(); i++ {
		entries := *c.largestEntries
		// 阈值默认为0，当大于0时，将过滤掉小于阈值的key
		if sizeFilter > 0 {
			if entries[i].Bytes > uint64(sizeFilter) {
				res = append(res, entries[i])
			}
		} else {
			res = append(res, entries[i])
		}
	}
	sort.Sort(sort.Reverse(entryHeap(res)))
	if num < len(res) {
		res = res[:num]
	}
	return res
}

// GetLargestKeyPrefixes from heap
func (c *Counter) GetLargestKeyPrefixes() []*PrefixEntry {
	res := []*PrefixEntry{}

	// get a copy of c.largestKeyPrefixes
	for i := 0; i < c.largestKeyPrefixes.Len(); i++ {
		entries := *c.largestKeyPrefixes
		res = append(res, entries[i])
	}
	sort.Sort(sort.Reverse(prefixHeap(res)))
	return res
}

// GetLenLevelCount from map
func (c *Counter) GetLenLevelCount() []*PrefixEntry {
	res := []*PrefixEntry{}

	// get a copy of lengthLevelBytes and lengthLevelNum
	for key := range c.lengthLevelBytes {
		res = append(res, &PrefixEntry{
			typeKey: key,
			Bytes:   c.lengthLevelBytes[key],
			Num:     c.lengthLevelNum[key], // map 读取不存在的 key 返回零值，不会 panic
			Db:     "", //对于lengthLevel(数量\内存)分布分析来说，db是无用的，因此这里直接赋空
		})
	}
	return res
}

func (c *Counter) countLargestEntries(e *decoder.Entry, num int) {
	heap.Push(c.largestEntries, e)
	l := c.largestEntries.Len()
	if l > num {
		heap.Pop(c.largestEntries)
	}
}

func (c *Counter) countByLength(e *decoder.Entry) {
	lengthLevelKey := typeKey{
		Type: e.Type,
		Key:  strconv.FormatUint(c.lengthLevel0, 10),
	}

	add := func(c *Counter, key typeKey, e *decoder.Entry) {
		c.lengthLevelBytes[key] += e.Bytes
		c.lengthLevelNum[key]++
	}

	numOfKey := e.NumOfElem
	lengthLevelStr := LengthLevelStr
	switch  {
	case numOfKey == 0:
		lengthLevelStr = LengthLevelStr
	case numOfKey > 0 && numOfKey <= c.lengthLevel0:
		lengthLevelStr = LengthLevelStr0
	case numOfKey > c.lengthLevel0 && numOfKey <= c.lengthLevel1:
		lengthLevelStr = LengthLevelStr1
	case numOfKey > c.lengthLevel1 && numOfKey <= c.lengthLevel2:
		lengthLevelStr = LengthLevelStr2
	case numOfKey > c.lengthLevel2 && numOfKey <= c.lengthLevel3:
		lengthLevelStr = LengthLevelStr3
	case numOfKey > c.lengthLevel3 && numOfKey <= c.lengthLevel4:
		lengthLevelStr = LengthLevelStr4
	case numOfKey > c.lengthLevel4:
		lengthLevelStr = LengthLevelStr5
	}
	lengthLevelKey.Key = lengthLevelStr
	add(c, lengthLevelKey, e)

}

func (c *Counter) countByType(e *decoder.Entry) {
	c.typeNum[e.Type]++
	c.typeBytes[e.Type] += e.Bytes
}

// 过期剩余时间分析，v1.1.5 add
func (c *Counter) countByExpire(e *decoder.Entry) {
    if e.Expiration >0 {
		// rdb的创建时间，是秒间戳，key的过期时间是毫秒时间戳
		// 转换成时间对象后，计算两个时间的差值
		diff := time.Unix(0, e.Expiration*int64(time.Millisecond)).Sub(time.Unix(c.config.Aux_Ctime, 0))
		// 将差值转换为小时数
		h := diff.Hours();
		expireStatStr := ""
		switch  {
		case h < 0:
			expireStatStr = ExpireStat1Str
		case h > 0 && h <= 1:
			expireStatStr = EExpireStat0_1hStr
		case h > 1 && h <= 3:
			expireStatStr = ExpireStat1_3hStr
		case h > 3 && h <= 12:
			expireStatStr = ExpireStat3_12hStr
		case h > 12 && h <= 24:
			expireStatStr = ExpireStat12_24hStr
		case h > 24 && h <= 24*3:
			expireStatStr = ExpireStat1_3dStr
		case h > 24*3 && h <= 24*7:
			expireStatStr = ExpireStat3_7dStr
		case h > 24*7:
			expireStatStr = ExpireStat7dStr
		}
		c.expireStatNum[expireStatStr]++
		c.expireStatBytes[expireStatStr] += e.Bytes
    } else {
		c.expireStatNum[ExpireStat0Str]++
		c.expireStatBytes[ExpireStat0Str] += e.Bytes
	}
}

// 按key的大小，进行区间分析，v1.1.5 add
func (c *Counter) countByByte(e *decoder.Entry) {
	bytesOfKey := e.Bytes
    tmpByteLevelStr := ByteLevelStr1k
	switch  {
	case bytesOfKey <= 1024:
		tmpByteLevelStr = ByteLevelStr1k
	case bytesOfKey > 1024 && bytesOfKey <= 10240:
		tmpByteLevelStr = ByteLevelStr10k
	case bytesOfKey > 10240 && bytesOfKey <= 102400:
		tmpByteLevelStr = ByteLevelStr100k
	case bytesOfKey > 102400 && bytesOfKey <= (1024*1024):
		tmpByteLevelStr = ByteLevelStr1M
	case bytesOfKey > (1024*1024) && bytesOfKey <= (1024*1024*10):
		tmpByteLevelStr = ByteLevelStr10M
	case bytesOfKey > (1024*1024*10) && bytesOfKey <= (1024*1024*100):
		tmpByteLevelStr = ByteLevelStr100M
	case bytesOfKey > (1024*1024*100):
		tmpByteLevelStr = ByteLevelStrAbove100M
	}
	c.byteLevelNum[tmpByteLevelStr]++
	c.byteLevelBytes[tmpByteLevelStr] += e.Bytes
}


func (c *Counter) countBySlot(e *decoder.Entry) {
	if len(e.Key) > 0 {
        slot := Slot(e.Key)
        stat := c.slotStats[slot]
        stat.Num++
        stat.Bytes += e.Bytes
        c.slotStats[slot] = stat // 写回
    }
}

// 传入一个entry，根据key名，通过分隔符得到前缀，然后对各前缀进行计数
func (c *Counter) countByKeyPrefix(e *decoder.Entry) {

	// 将key名字中的所有数字替换（通常为id号）为0
	k := strings.Map(func(c rune) rune {
		if c >= 48 && c <= 57 { //48 == "0" 57 == "9"
			return '*' // -1则是删除数字,'0'、'*'替换成0或*
		}
		return c
	}, e.Key)

	// 1.将key名字进行分割，得到所有前缀字符串
	prefixes := getPrefixes(k, c.config.Separators)

	key := typeKey{
		Type: e.Type,
	}

	// 2.遍历前缀，对其计数
	for _, prefixStr := range prefixes {
		if len(prefixStr) == 0 {
			continue
		}

		// ⭐ 关键优化：存储前缀的独立副本，不再引用 'k'
		key.Key = strings.Clone(prefixStr)
		stat, ok := c.keyPrefixStats[key]
		if ok {
			// 已存在，直接更新统计
			stat.Bytes += e.Bytes
			stat.Num++
			stat.addDB(e.Db) //将当前处理条目所属的db编号（e.Db）记录到该前缀的统计信息中
		} else {
			// 不存在
			stat = &prefixStat{
				Bytes:  e.Bytes,
				Num:    1,
				dbMask:  0,
			}
			stat.addDB(e.Db)
			c.keyPrefixStats[key] = stat
		}

	}

	// 增加动态缩容机制，防止创建海量的前缀对象，导致因内存不足而执行不完
	// 3.如果不开存储所有前缀，并且前缀数量（也可以用 c.keyPrefixNum[key]）超过容器的最大容量时，则进行缩容
	// 你可以使用切片排序代替堆方案（calcuLargestKeyPrefix2） 或 使用优化后的堆方案（calcuLargestKeyPrefix3）
	if !c.config.StoreAllPrefixes && len(c.keyPrefixStats) > c.config.PrefixContainerMaxCapacity {
		c.calcuLargestKeyPrefix(c.config.PrefixPreShrinkNum)
	}

}

// get largest prefixes
// 方案1，采用最小堆方案
// 功能：从当前 Map 中保留字节数最大的前 num 个前缀，其余删除
// 特点：合并单张 Map + 边遍历边淘汰，峰值内存低
func (c *Counter) calcuLargestKeyPrefix2(num int) {
	// 1. 初始化最小堆（容量 num）
	h := &prefixHeap{}
	heap.Init(h)

	// 2. 遍历 Map，边遍历边淘汰
	for key, stat := range c.keyPrefixStats {
		// 构建堆条目（暂不填充 Db）
		entry := &PrefixEntry{
			typeKey: key,
			Bytes:   stat.Bytes,
			Num:     stat.Num,
			Db:      "", // 延迟填充，避免在循环中反复调用 dbSetToString
		}
		if h.Len() < num {
			heap.Push(h, entry)
			// 保留在 map 中
		} else {
			// 堆顶是最小 Bytes 的条目，如果当前条目比堆顶大
			if entry.Bytes > (*h)[0].Bytes {
				// 淘汰堆顶：// 弹出堆顶（最小的），从map中删除被淘汰的，再推入新元素
				popped := heap.Pop(h).(*PrefixEntry)
				delete(c.keyPrefixStats, popped.typeKey)
				heap.Push(h, entry)
			} else {
				// 当前条目比堆顶小或相等，淘汰当前条目
				delete(c.keyPrefixStats, key)
				// entry 对象可被 GC 回收
			}
		}
	}

	// 3. 缩容后，map 中只保留了堆中的条目（数量 ≤ num）
	//    此时填充堆中条目的 Db 字段（基于保留的 map 中的 DbSet）
	for _, entry := range *h {
		if stat, ok := c.keyPrefixStats[entry.typeKey]; ok {
			entry.Db = dbMaskToString(stat.dbMask) // 仅保留的前缀才转换 Db
		}
	}

	// 4. 保存堆供外部使用
	c.largestKeyPrefixes = h

}

// 方案1的改进
// calcuLargestKeyPrefix 保留字节数最大的前 num 个前缀，同时回收被淘汰的统计对象
// 改进点：① 遍历时不再 delete；② 结束后重建 Map 释放底层大桶
func (c *Counter) calcuLargestKeyPrefix(num int) {
	// 1. 初始化最小堆（容量 num）
	h := &prefixHeap{}
	heap.Init(h)

	// 2. 单次遍历Map，只进堆，绝不删除原 Map（避免写屏障与迭代干扰）
	for key, stat := range c.keyPrefixStats {
		entry := &PrefixEntry{
			typeKey: key,
			Bytes:   stat.Bytes,
			Num:     stat.Num,
			Db:      "", // 延迟填充，避免在循环中反复调用 dbSetToString
		}

		if h.Len() < num {
			heap.Push(h, entry)
		} else if entry.Bytes > (*h)[0].Bytes { // 比堆顶大才替换
			heap.Pop(h)      // 弹出最小的
			heap.Push(h, entry)
		}
		// 小于堆顶的条目直接被丢弃（Go GC 会在后续回收 entry）
	}

	// 3. 关键改善：重建小 Map（从堆中取出所有元素，重建新 Map，容量精确），释放旧的大 Map
	newMap := make(map[typeKey]*prefixStat, h.Len())
	for _, entry := range *h {
		// 从旧 Map 中获取 stat 指针（旧 Map 尚未被覆盖，可安全读取）
		if stat, ok := c.keyPrefixStats[entry.typeKey]; ok {
			entry.Db = dbMaskToString(stat.dbMask) // 仅保留的前缀才转换 Db
			newMap[entry.typeKey] = stat        // 复用原有 stat 对象，避免拷贝
		}
	}

	// 4. 原子替换：旧 Map 失去引用，等待 GC 回收（底层大桶被释放）
	c.keyPrefixStats = newMap
	c.largestKeyPrefixes = h
}

// 方案3，用切片排序替代堆
func (c *Counter) calcuLargestKeyPrefix3(num int) {
	// 1. 提取所有统计到切片
	type item struct {
		key  typeKey
		stat *prefixStat
	}
	items := make([]item, 0, len(c.keyPrefixStats))
	for key, stat := range c.keyPrefixStats {
		items = append(items, item{key: key, stat: stat})
	}

	// 2. 立即释放旧 Map （大池子），让 GC 回收桶
	c.keyPrefixStats = nil

	// 3. 按 Bytes 降序排序
	sort.Slice(items, func(i, j int) bool {
		return items[i].stat.Bytes > items[j].stat.Bytes
	})

	// 4. 只保留前 num 个
	if len(items) > num {
		items = items[:num]
	}

	// 5. 重建新 Map（缩容后的map）
	c.keyPrefixStats = make(map[typeKey]*prefixStat, len(items))
	for _, it := range items {
		c.keyPrefixStats[it.key] = it.stat
	}

	// 6. 更新 largestKeyPrefixes，因 GetLargestKeyPrefixes from heap
	// if c.largestKeyPrefixes != nil {
	// 	// 将 items 转为 []*prefixStat 并赋值给堆（需要调整堆的 Less 方法）
	// 	// 直接将排序后的切片复用为堆底层（要求 prefixHeap 底层为 []*PrefixEntry）
	// 	// 将 items 转换为 []*PrefixEntry，并赋值给堆
	//	topEntries := make([]*PrefixEntry, 0, len(items))
	//	for _, it := range items {
	// 		topEntries = append(topEntries, &PrefixEntry{
    //             typeKey: it.key,
    //             Bytes:   it.stat.Bytes,
    //             Num:     it.stat.Num,
    //             Db:      dbSetToString(it.stat.DbSet),
    //         })
    //     }
    //     /*c.largestKeyPrefixes = prefixHeap(topEntries)
    //     heap.Init(c.largestKeyPrefixes)
    // }
	// 上面的（指向指针的内容）和下面的（指向新指针）用哪个都行
	topList := make([]*PrefixEntry, len(items))
    for i, it := range items {
            topList[i] = &PrefixEntry{
                typeKey: it.key,
                Bytes:   it.stat.Bytes,
                Num:     it.stat.Num,
				Db:      dbMaskToString(it.stat.dbMask),
            }
    }
    h := prefixHeap(topList)
    c.largestKeyPrefixes = &h
    heap.Init(c.largestKeyPrefixes)

}


// 辅助函数：将 DbSet 转为逗号字符串
func dbSetToString(dbSet map[int]struct{}) string {
    if len(dbSet) == 0 {
        return ""
    }
    ids := make([]int, 0, len(dbSet))
    for id := range dbSet {
        ids = append(ids, id)
    }
    sort.Ints(ids) // 可选，保持稳定
    var b strings.Builder
    for i, id := range ids {
        if i > 0 {
            b.WriteByte(',')
        }
        b.WriteString(strconv.Itoa(id))
    }
    return b.String()
}

// 辅助函数：将逗号字符串解析为 DbSet
func parseDbSet(s string) map[int]struct{} {
    if s == "" {
        return nil
    }
    parts := strings.Split(s, ",")
    m := make(map[int]struct{}, len(parts))
    for _, p := range parts {
        if id, err := strconv.Atoi(p); err == nil {
            m[id] = struct{}{}
        }
    }
    return m
}


// -------------------- 类型定义 --------------------
type entryHeap []*decoder.Entry

func (h entryHeap) Len() int {
	return len(h)
}
func (h entryHeap) Less(i, j int) bool {
	return h[i].Bytes < h[j].Bytes
}
func (h entryHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *entryHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

func (h *entryHeap) Push(e interface{}) {
	*h = append(*h, e.(*decoder.Entry))
}

// 前缀分析
// -------------------- 类型定义 --------------------

// typeKey 作为 map 的复合键
type typeKey struct {
	Type string
	Key  string
}

type prefixHeap []*PrefixEntry

// PrefixEntry 堆元素（ record value by prefix），包含类型键、统计值和对应统计对象
type PrefixEntry struct {
	typeKey
	Bytes uint64
	Num   uint64
	Db    string // 逗号分隔的 db 列表（仅为输出方便）
}

// ======================= 前缀统计信息 =======================
// type prefixStat struct {
// 	Bytes uint64          // 累计字节数
// 	Num   uint64          // 累计命中次数
// 	DbSet map[int]struct{} // 记录该前缀出现过的 DB 编号（去重）
// }

// prefixStat 统计信息，使用 uint64 位掩码存储 DB 编号（支持 0~63）
type prefixStat struct {
	Bytes   uint64
	Num     uint64
	dbMask  uint64 // 位掩码，bit i 表示 DB i 出现过
}

// 辅助函数：addDB 添加 DB 编号（0~63）
func (s *prefixStat) addDB(db int) {
	if db >= 0 && db < 64 {
		s.dbMask |= 1 << uint(db)
	}
}

// 辅助函数：dbSetToString 将位掩码转为逗号分隔的字符串
func dbMaskToString(mask uint64) string {
	if mask == 0 {
		return ""
	}
	var ids []int
	for i := 0; i < 64; i++ {
		if mask&(1<<uint(i)) != 0 {
			ids = append(ids, i)
		}
	}
	var b strings.Builder
	for i, id := range ids {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Itoa(id))
	}
	return b.String()
}


func (h prefixHeap) Len() int {
	return len(h)
}
func (h prefixHeap) Less(i, j int) bool {
	if h[i].Bytes < h[j].Bytes {
		return true
	} else if h[i].Bytes == h[j].Bytes {
		if h[i].Num < h[j].Num {
			return true
		} else if h[i].Num == h[j].Num {
			if h[i].Key > h[j].Key {
				return true
			}
		}
	}
	return false

}
func (h prefixHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *prefixHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

func (h *prefixHeap) Push(k interface{}) {
	*h = append(*h, k.(*PrefixEntry))
}

func appendIfMissing(slice []int, i int) []int {
	for _, ele := range slice {
		if ele == i {
			return slice
		}
	}
	return append(slice, i)
}

// 提前所有前缀用到的去重方法
func removeDuplicatesUnordered(elements []string) []string {
	encountered := map[string]bool{}

	// Create a map of all unique elements.
	for v := range elements {
		encountered[elements[v]] = true
	}

	// Place all keys from the map into a slice.
	result := []string{}
	for key := range encountered {
		result = append(result, key)
	}
	return result
}

// getPrefixes 高效提取所有前缀
func getPrefixes(s, sep string) []string {
	res := []string{}
	sepIdx := strings.IndexAny(s, sep)
	if sepIdx < 0 {
		res = append(res, s)
	}
	for sepIdx > -1 {
		r := s[:sepIdx+1]
		if len(res) > 0 {
			r = res[len(res)-1] + s[:sepIdx+1]
		}
		res = append(res, r)
		s = s[sepIdx+1:]
		sepIdx = strings.IndexAny(s, sep)
	}
	// Trim all suffix of separators
	for i := range res {
		for hasAnySuffix(res[i], sep) {
			res[i] = res[i][:len(res[i])-1]
		}
	}
	// ⭐ 可以移除去重调用，直接返回（重复项由外部 map 去重）
    // 之前：res = removeDuplicatesUnordered(res)
	// res = removeDuplicatesUnordered(res)
	return res
}

func hasAnySuffix(s, suffix string) bool {
	for _, c := range suffix {
		if strings.HasSuffix(s, string(c)) {
			return true
		}
	}
	return false
}

// support for sorting of slots
type SlotEntry struct {
	Slot int
	Size uint64
}

type slotHeap []*SlotEntry

func (h slotHeap) Len() int {
	return len(h)
}
func (h slotHeap) Less(i, j int) bool {
	return h[i].Size > h[j].Size
}
func (h slotHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *slotHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

func (h *slotHeap) Push(e interface{}) {
	*h = append(*h, e.(*SlotEntry))
}
