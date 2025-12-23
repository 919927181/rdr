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

package decoder

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/919927181/rdb"
	"github.com/919927181/rdb/nopdecoder"
)

// Entry is info of a redis recored
type Entry struct {
	Key                string
	Bytes              uint64
	Type               string
	NumOfElem          uint64
	LenOfLargestElem   uint64
	FieldOfLargestElem string
	Db                 int
	Encoding           string
	Expiration         string
}

// Decoder decode rdb file
type Decoder struct {
	Entries chan *Entry
	m       MemProfiler

	usedMem int64
	ctime   int64
	//count   int
	rdbVer int //rdb file version
	Db     int

	currentInfo  *rdb.Info
	currentEntry *Entry

	nopdecoder.NopDecoder
}

// NewDecoder new a rdb decoder
func NewDecoder() *Decoder {
	return &Decoder{
		Entries: make(chan *Entry, 1024),
		m:       MemProfiler{},
	}
}

func (d *Decoder) sendEntry() {
	d.Entries <- d.currentEntry
	d.currentEntry = nil
}

func (d *Decoder) GetTimestamp() int64 {
	return d.ctime
}

func (d *Decoder) GetUsedMem() int64 {
	return d.usedMem
}

func (d *Decoder) StartRDB(ver int) {
	d.rdbVer = ver
}

func (d *Decoder) StartDatabase(n int) {
	d.Db = n
}

func (d *Decoder) Aux(key, value []byte) {
	switch string(key) {
	case "ctime":
		{
			n, err := strconv.ParseInt(string(value), 10, 64)
			if err != nil {
				fmt.Fprintln(os.Stderr, "ParseInt(", string(value), "):", err)
			}
			d.ctime = n
		}

	case "used-mem":
		{
			n, err := strconv.ParseInt(string(value), 10, 64)
			if err != nil {
				fmt.Fprintln(os.Stderr, "ParseInt(", string(value), "):", err)
			}
			d.usedMem = n
		}

	}
}

func (d *Decoder) StartStream(key []byte, cardinality, expiry int64, info *rdb.Info) {
	keyStr := string(key)
	bytes := d.m.TopLevelObjOverhead(key, expiry)
	bytes += d.m.StreamOverhead()
	bytes += d.m.SizeofStreamRadixTree(uint64(cardinality))

	expiryStr := ""
	if expiry > 0 {
		expiryStr = time.Unix(0, expiry*int64(time.Millisecond)).Format("2006-01-02 15:04:05")
	}

	d.currentInfo = info
	d.currentEntry = &Entry{
		Key:              keyStr,
		Bytes:            bytes,
		Type:             "stream",
		Encoding:         info.Encoding,
		NumOfElem:        0,
		LenOfLargestElem: 0,
		Expiration:       expiryStr,
		Db:               d.Db,
	}
}

func (d *Decoder) Xadd(key, id, listpack []byte) {
	e := d.currentEntry
	e.Bytes += d.m.mallocOverhead(uint64(len(listpack)))
}

func (d *Decoder) EndStream(key []byte, items uint64, lastEntryID string, cgroupsData rdb.StreamGroups) {
	e := d.currentEntry

	for _, cg := range cgroupsData {
		pendingLength := uint64(len(cg.Pending))
		e.Bytes += d.m.SizeofStreamRadixTree(pendingLength)
		e.Bytes += d.m.StreamNACK(pendingLength)

		for _, c := range cg.Consumers {
			e.Bytes += d.m.StreamConsumer(c.Name)
			pendingLength := uint64(len(cg.Pending))
			e.Bytes += d.m.SizeofStreamRadixTree(pendingLength)
		}
	}

	d.sendEntry()
}

// Set is called once for each string key.
func (d *Decoder) Set(key, value []byte, expiry int64, info *rdb.Info) {
	keyStr := string(key)
	bytes := d.m.TopLevelObjOverhead(key, expiry)
	bytes += d.m.SizeofString(value)

	expiryStr := ""
	if expiry > 0 {
		expiryStr = time.Unix(0, expiry*int64(time.Millisecond)).Format("2006-01-02 15:04:05")
	}

	e := &Entry{
		Key:        keyStr,
		Bytes:      bytes,
		Type:       "string",
		Encoding:   info.Encoding,
		NumOfElem:  d.m.ElemLen(value),
		Expiration: expiryStr,
		Db:         d.Db,
	}
	d.Entries <- e
}

// StartHash is called at the beginning of a hash.
// Hset will be called exactly length times before EndHash.
func (d *Decoder) StartHash(key []byte, length, expiry int64, info *rdb.Info) {
	keyStr := string(key)

	bytes := d.m.TopLevelObjOverhead(key, expiry)

	if info.Encoding == "hashtable" {
		bytes += d.m.HashTableOverHead(uint64(length))
	} else if info.Encoding == "listpack" || info.Encoding == "ziplist" || info.Encoding == "zipmap" {
		if info.SizeOfValue > 0 {
			bytes += uint64(info.SizeOfValue)
        }
	} else {
		panic(fmt.Sprintf("unexpected size(0) or encoding:%s", info.Encoding))
	}


	expiryStr := ""
	if expiry > 0 {
		expiryStr = time.Unix(0, expiry*int64(time.Millisecond)).Format("2006-01-02 15:04:05")
	}

	d.currentInfo = info
	d.currentEntry = &Entry{
		Key:        keyStr,
		Bytes:      bytes,
		Type:       "hash",
		Encoding:   info.Encoding,
		NumOfElem:  uint64(length),
		Expiration: expiryStr,
		Db:         d.Db,
	}
}

// Hset is called once for each field=value pair in a hash.
func (d *Decoder) Hset(key, field, value []byte) {
	e := d.currentEntry

	lenOfElem := d.m.ElemLen(field) + d.m.ElemLen(value)
	if lenOfElem > e.LenOfLargestElem {
		e.FieldOfLargestElem = string(field)
		e.LenOfLargestElem = lenOfElem
	}
	// StartHash中listpack类型的大小是整个的，包含了key和所有内部元素的大小
	if d.currentInfo.Encoding == "hashtable" {
		e.Bytes += d.m.SizeofString(field)
		e.Bytes += d.m.SizeofString(value)
		e.Bytes += d.m.HashTableEntryOverHead()

		if d.rdbVer < 10 {
			e.Bytes += 2 * d.m.RobjOverHead()
		}
	}
}

// EndHash is called when there are no more fields in a hash.
func (d *Decoder) EndHash(key []byte) {
	d.sendEntry()
}

// StartSet is called at the beginning of a set.
// Sadd will be called exactly cardinality times before EndSet.
func (d *Decoder) StartSet(key []byte, cardinality, expiry int64, info *rdb.Info) {
	//d.StartHash(key, cardinality, expiry, info)

	keyStr := string(key)
	bytes := d.m.TopLevelObjOverhead(key, expiry)

	//IntSetEncoding
	if info.Encoding == "intset" && info.SizeOfValue > 0 {
		bytes += uint64(info.SizeOfValue)
	} else if info.Encoding == "hashtable" {
		bytes += d.m.HashTableOverHead(uint64(cardinality))
	} else if info.Encoding == "listpack" && info.SizeOfValue > 0 {
		bytes += uint64(info.SizeOfValue)
	} else {
		panic(fmt.Sprintf("unexpected size(0) or encoding:%s", info.Encoding))
	}

	expiryStr := ""
	if expiry > 0 {
		expiryStr = time.Unix(0, expiry*int64(time.Millisecond)).Format("2006-01-02 15:04:05")
	}

	d.currentInfo = info
	d.currentEntry = &Entry{
		Key:        keyStr,
		Bytes:      bytes,
		Type:       "set",
		Encoding:   info.Encoding,
		NumOfElem:  uint64(cardinality),
		Expiration: expiryStr,
		Db:         d.Db,
	}

}

// Sadd is called once for each member of a set.
func (d *Decoder) Sadd(key, member []byte) {
	e := d.currentEntry
	lenOfElem := d.m.ElemLen(member)
	if lenOfElem > e.LenOfLargestElem {
		e.FieldOfLargestElem = string(member)
		e.LenOfLargestElem = lenOfElem
	}
	// StartSet中非hashtable类型的都是加的整个key的大小（包含了key和所有内部元素的大小）
	if d.currentInfo.Encoding == "hashtable" {
		e.Bytes += d.m.SizeofString(member)
		e.Bytes += d.m.HashTableEntryOverHead()

		if d.rdbVer < 10 {
			e.Bytes += d.m.RobjOverHead()
		}
	}
}

// EndSet is called when there are no more fields in a set.
// Same as EndHash
func (d *Decoder) EndSet(key []byte) {
	d.sendEntry()
}

// StartList is called at the beginning of a list.
// Rpush will be called exactly length times before EndList.
// If length of the list is not known, then length is -1
func (d *Decoder) StartList(key []byte, length, expiry int64, info *rdb.Info) {
	keyStr := string(key)

	d.currentInfo = info
	bytes := d.m.TopLevelObjOverhead(key, expiry)

	expiryStr := ""
	if expiry > 0 {
		expiryStr = time.Unix(0, expiry*int64(time.Millisecond)).Format("2006-01-02 15:04:05")
	}

	//bug here length would be -4 if it is quicklist2
	//bytes += d.m.QuickList2OverHead() + (d.m.RobjOverHead() * uint64(length))

	//bug here length would be -1 if it is quicklist
	//bytes += d.m.RobjOverHead() * uint64(length)
	d.currentEntry = &Entry{
		Key:        keyStr,
		Bytes:      bytes,
		Type:       "list",
		Encoding:   info.Encoding,
		NumOfElem:  0,
		Expiration: expiryStr,
		Db:         d.Db,
	}
}

// Rpush is called once for each value in a list.
func (d *Decoder) Rpush(key, value []byte) {
	//keyStr := string(key)
	e := d.currentEntry
	e.NumOfElem++

	switch d.currentInfo.Encoding {
	case "quicklist":
		e.Bytes += d.m.ZipListEntryOverHead(value)
	case "ziplist":
		e.Bytes += d.m.ZipListEntryOverHead(value)

	case "linkedlist":
		sizeInlist := uint64(0)
		if _, err := strconv.ParseInt(string(value), 10, 32); err != nil {
			sizeInlist = d.m.SizeofString(value)
		}

		e.Bytes += d.m.LinkedListEntryOverHead()
		e.Bytes += sizeInlist

		if d.rdbVer < 10 {
			e.Bytes += d.m.RobjOverHead()
		}

	case "quicklist2":
		if _, err := strconv.ParseInt(string(value), 10, 32); err != nil {
			e.Bytes += d.m.SizeofString(value) //参考 github.com/hdt3213/rdb/memprofiler
		}

	case "listpack":
		e.Bytes += 0

	default:
		panic(fmt.Sprintf("unknown encoding:%s", d.currentInfo.Encoding))
	}

	lenOfElem := d.m.ElemLen(value)
	if lenOfElem > e.LenOfLargestElem {
		e.FieldOfLargestElem = string(value)
		e.LenOfLargestElem = lenOfElem
	}
}

// EndList is called when there are no more values in a list.
func (d *Decoder) EndList(key []byte) {
	e := d.currentEntry

	switch d.currentInfo.Encoding {
	case "quicklist":
		e.Bytes += d.m.QuickListOverHead(d.currentInfo.Zips)
		e.Bytes += d.m.ZipListHeaderOverHead() * d.currentInfo.Zips

	case "ziplist":
		e.Bytes += d.m.ZipListHeaderOverHead()

	case "linkedlist":
		e.Bytes += d.m.LinkedListOverHead()

	case "quicklist2":
		e.Bytes += 0 //已加到startlist中。后期写计算方法时可参考 github.com/hdt3213/rdb/memprofiler

	case "listpack":
		e.Bytes += d.m.QuickList2OverHead()
		e.Bytes += d.m.RobjOverHead() * d.currentInfo.Zips
		//fmt.Printf("2、quicklist2 OverHead use memory  %d for string key %s\n", int(d.m.QuickList2OverHead() + d.m.RobjOverHead() * d.currentInfo.Zips), string(key))

		if d.currentInfo.SizeOfValue > 0 {
			e.Bytes += uint64(d.currentInfo.SizeOfValue)
			//fmt.Printf("3、SizeOfValue use memory  %d for string key %s\n", uint64(d.currentInfo.SizeOfValue), string(key))
		}

	default:
		panic(fmt.Sprintf("unknown encoding:%s", d.currentInfo.Encoding))
	}

	d.sendEntry()
}

// StartZSet is called at the beginning of a sorted set.
// Zadd will be called exactly cardinality times before EndZSet.
func (d *Decoder) StartZSet(key []byte, cardinality, expiry int64, info *rdb.Info) {
	keyStr := string(key)

	bytes := d.m.TopLevelObjOverhead(key, expiry)
	d.currentInfo = info

	if info.Encoding == "ziplist" && info.SizeOfValue > 0 {
		bytes += uint64(info.SizeOfValue)
	} else if info.Encoding == "skiplist" {
		bytes += d.m.SkipListOverHead(uint64(cardinality))
	} else if info.Encoding == "listpack" && info.SizeOfValue > 0 {
		bytes += uint64(info.SizeOfValue)
	} else {
		panic(fmt.Sprintf("unexpected size(0) or encoding:%s", info.Encoding))
	}

	expiryStr := ""
	if expiry > 0 {
		expiryStr = time.Unix(0, expiry*int64(time.Millisecond)).Format("2006-01-02 15:04:05")
	}

	d.currentEntry = &Entry{
		Key:        keyStr,
		Bytes:      bytes,
		Type:       "sortedset", //Sorted Set 和 zset 指的是同一个东西
		Encoding:   info.Encoding,
		NumOfElem:  uint64(cardinality),
		Expiration: expiryStr,
		Db:         d.Db,
	}
}

// Zadd is called once for each member of a sorted set.
func (d *Decoder) Zadd(key []byte, score float64, member []byte) {
	e := d.currentEntry
	lenOfElem := d.m.ElemLen(member)
	if lenOfElem > e.LenOfLargestElem {
		e.FieldOfLargestElem = string(member)
		e.LenOfLargestElem = lenOfElem
	}
	// StartZSet中，仅skiplist类型加了SkipListOverHead，其他类型都是加的整个类型的bytes，包含了key和所有内部元素的大小
	if d.currentInfo.Encoding == "skiplist" {
		e.Bytes += 8 // sizeof(score)
		e.Bytes += d.m.SizeofString(member)
		e.Bytes += d.m.SkipListEntryOverHead()

		if d.rdbVer < 10 {
			e.Bytes += d.m.RobjOverHead()
		}
	}
}

// EndZSet is called when there are no more members in a sorted set.
func (d *Decoder) EndZSet(key []byte) {
	d.sendEntry()
}

// EndRDB is called when parsing of the RDB file is complete.
func (d *Decoder) EndRDB() {
	close(d.Entries)
}
