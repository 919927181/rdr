// Package rdb implements parsing and encoding of the Redis RDB file format.
package rdb

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strconv"

	"github.com/919927181/rdb/core/structure"
	"github.com/919927181/rdb/core/types"
	"github.com/919927181/rdb/crc64"
	"github.com/919927181/rdb/internal/log"
	"github.com/juju/errors"
)

type Info struct {
	Encoding    string
	Idle        uint64
	Freq        int
	SizeOfValue int
	Zips        uint64
	ListPacks   uint64
}

type StreamPendingEntry struct {
	ID            *StreamId
	DeliveryTime  uint64
	DeliveryCount uint64
	IsNack        bool // 新增：标识是否为 NACK zone 条目（无消费者）
}

type StreamConsumerPendingEntry struct {
	ID []byte
}

type StreamConsumerData struct {
	Name     []byte
	SeenTime uint64
	Pending  []*StreamConsumerPendingEntry
	ActiveTime uint64
}

type StreamGroup struct {
	Name        []byte
	LastEntryId string
	Pending     []*StreamPendingEntry
	Consumers   []*StreamConsumerData
}

type StreamGroups []*StreamGroup

// A Decoder must be implemented to parse a RDB file.
type Decoder interface {
	// StartRDB is called when parsing of a valid RDB file starts.
	StartRDB(ver int)
	// StartDatabase is called when database n starts.
	// Once a database starts, another database will not start until EndDatabase is called.
	StartDatabase(n int)
	// AUX field
	Aux(key, value []byte)
	// ResizeDB hint
	ResizeDatabase(dbSize, expiresSize uint32)
	// Set is called once for each string key.
	Set(key, value []byte, expiry int64, info *Info)
	// StartHash is called at the beginning of a hash.
	// Hset will be called exactly length times before EndHash.
	StartHash(key []byte, length, expiry int64, info *Info)
	// Hset is called once for each field=value pair in a hash.
	Hset(key, field, value []byte)
	// EndHash is called when there are no more fields in a hash.
	EndHash(key []byte)
	// StartSet is called at the beginning of a set.
	// Sadd will be called exactly cardinality times before EndSet.
	StartSet(key []byte, cardinality, expiry int64, info *Info)
	// Sadd is called once for each member of a set.
	Sadd(key, member []byte)
	// EndSet is called when there are no more fields in a set.
	EndSet(key []byte)
	// StartStream is called at the beginning of a stream.
	// Xadd will be called exactly length times before EndStream.
	StartStream(key []byte, cardinality, expiry int64, info *Info)
	// Xadd is called once for each id in a stream.
	Xadd(key, id, listpack []byte)
	// EndHash is called when there are no more fields in a hash.
	EndStream(key []byte, items uint64, lastEntryID string, cgroupsData StreamGroups)
	// StartList is called at the beginning of a list.
	// Rpush will be called exactly length times before EndList.
	// If length of the list is not known, then length is -1
	StartList(key []byte, length, expiry int64, info *Info)
	// Rpush is called once for each value in a list.
	// rdb v1.0.8增加NodeEncodings是为了支持redis7+的Quicklist2，qucklist2中节点有两种编码1和2，其他数据类型传0
	Rpush(key, value []byte, NodeEncodings uint64)
	// EndList is called when there are no more values in a list.
	EndList(key []byte )
	// StartZSet is called at the beginning of a sorted set.
	// Zadd will be called exactly cardinality times before EndZSet.
	StartZSet(key []byte, cardinality, expiry int64, info *Info)
	// Zadd is called once for each member of a sorted set.
	Zadd(key []byte, score float64, member []byte)
	// EndZSet is called when there are no more members in a sorted set.
	EndZSet(key []byte)
	// EndDatabase is called at the end of a database.
	EndDatabase(n int)
	// EndRDB is called when parsing of the RDB file is complete.
	EndRDB()
}

// 验证rdb文件的合法性
func verifyDump(d []byte) error {
	if len(d) < 10 {
		return fmt.Errorf("rdb: invalid dump length")
	}

	if binary.LittleEndian.Uint64(d[len(d)-8:]) != crc64.Digest(d[:len(d)-8]) {
		return fmt.Errorf("rdb: invalid CRC checksum")
	}

	return nil
}

// Decode parses a RDB file from r and calls the decode hooks on d.
func Decode(r io.Reader, d Decoder) error {
	decoder := &decode{d, make([]byte, 8), bufio.NewReader(r), 0, 0, nil, 0, false}
	return decoder.decode() //传指针，用指针作为receiver
}

// DecodeDump a byte slice from the Redis DUMP command. The dump does not contain the
// database, key or expiry, so they must be included in the function call (but
// can be zero values).
func DecodeDump(dump []byte, db int, key []byte, expiry int64, d Decoder) error {
	err := verifyDump(dump)
	if err != nil {
		return errors.Trace(err)
	}

	decoder := &decode{d, make([]byte, 8), bytes.NewReader(dump[1:]), 0, 0, nil, 0, false}
	decoder.event.StartRDB(0)
	decoder.event.StartDatabase(db)

	err = decoder.readObject(key, ValueType(dump[0]), expiry)

	decoder.event.EndDatabase(db)
	decoder.event.EndRDB()
	return errors.Trace(err)
}

type byteReader interface {
	io.Reader
	io.ByteReader
}

type decode struct {
	event  Decoder
	intBuf []byte
	r      byteReader

	lruIdle uint64
	lfuFreq int

	info       *Info
	rdbVersion int
	isValkey   bool
}

// ValueType of redis type
type ValueType byte

// types value
// string: TypeString
// list: TypeList, TypeListZipList, TypeListQuickList, TypeListQuickList2
// set: TypeSet, TypeSetIntSet, TypeSetListPack
// Sorted Set（zset）: TypeZSet, TypeZSet2, TypeZSetZipList, TypeZSetListPack
// hash: TypeHash, TypeHashZipMap, TypeHashZipList, TypeHashListPack, TypeHashMetadataPreGa, TypeHashListPackExPre, TypeHashMetaData, TypeHashListPackEx
// 注：Redis7.0开始使用listpack替代了ziplist，小于阈值时使用listpack
const (
	TypeString  ValueType = 0 // RDB_TYPE_STRING
	TypeList    ValueType = 1
	TypeSet     ValueType = 2
	TypeZSet    ValueType = 3
	TypeHash    ValueType = 4 // RDB_TYPE_HASH
	TypeZSet2   ValueType = 5 // ZSET version 2 with doubles stored in binary.
	TypeModule  ValueType = 6 // RDB_TYPE_MODULE
	TypeModule2 ValueType = 7 // RDB_TYPE_MODULE2 Module value with annotations for parsing without the generating module being loaded.

	// Object types for encoded objects.
	TypeHashZipMap      ValueType = 9
	TypeListZipList     ValueType = 10
	TypeSetIntSet       ValueType = 11
	TypeZSetZipList     ValueType = 12
	TypeHashZipList     ValueType = 13
	TypeListQuickList   ValueType = 14 // RDB_TYPE_LIST_QUICKLIST
	TypeStreamListPacks ValueType = 15 // RDB_TYPE_STREAM_LISTPACKS，

	//rdb v1.0.5 add for redis7+，注：Redis Stream 主要用于消息队列，虽然我们一般不会用它，参考 github.com/linyue515/rdr 做了支持，后期有精力时间时再参考RedisShake进行梳理
	TypeHashListPack     ValueType = 16 // RDB_TYPE_HASH_ZIPLIST
	TypeZSetListPack     ValueType = 17 // RDB_TYPE_ZSET_LISTPACK
	TypeListQuickList2   ValueType = 18 // DB_TYPE_LIST_QUICKLIST_2 https://github.com/redis/redis/pull/9357
	TypeStreamListPacks2 ValueType = 19 // RDB_TYPE_STREAM_LISTPACKS2
	TypeSetListPack      ValueType = 20 // RDB_TYPE_SET_LISTPACK
	TypeStreamListPacks3 ValueType = 21 // RDB_TYPE_STREAM_LISTPACKS_3

	// https://github.com/redis/redis/pull/13391
	TypeHashMetaDataPreGa22 ValueType = 22 // Redis use RDB_TYPE_HASH_METADATA_PRE_GA, Valkey9.0 use RDB_TYPE_HASH_2
	TypeHashListPackExPre23 ValueType = 23 // RDB_TYPE_HASH_LISTPACK_EX_PRE_GA
	TypeHashMetaData24      ValueType = 24 // RDB_TYPE_HASH_METADATA /* Hash with HFEs. Attach min TTL at start */
	TypeHashListPackEx25    ValueType = 25 // RDB_TYPE_HASH_LISTPACK_EX

	TypeStreamListPacks4 ValueType = 26 // Stream with IDMP support (RDB 13, stream_v4)
	TypeStreamListPacks5 ValueType = 27 // Stream with XNACK support (NACKed entries) (RDB 14, stream_v5)

	TypeArray             ValueType = 28 /* Array data type, RDB 15*/
    TypeHashTmplLp        ValueType = 29 /* TMPL_LP, self-contained (DUMP): [count][f0]...[fN-1][lp_blob] */
    TypeHashTmplLpRRF     ValueType = 30 /* TMPL_LP, with template ref (RDB save): raw lp blob, first entry is tid */
    TypeHashTmplArray     ValueType = 31 /* TMPL_ARRAY, self-contained (DUMP): [count][f0][v0]...[fN-1][vN-1] */
	TypeHashTmplArrayREF  ValueType = 32 /* TMPL_ARRAY, with template ref (RDB save): [tid][v0]...[vN-1] */
    TypeGcra  ValueType = 33    /* GCRA object 从 Redis 8.8 版本开始，内置了基于 GCRA 算法的限流器*/
)

// 在checkHeader中判断rdb版本，我已注释掉判断
// const (
// 	maxVersionRedis = 16
// 	maxVersionValkey = 80  //Valkey 9.0
// )

const (
	EB_EXPIRE_TIME_MAX     int64 = 0x0000FFFFFFFFFFFF
	EB_EXPIRE_TIME_INVALID int64 = EB_EXPIRE_TIME_MAX + 1
	HFE_MAX_ABS_TIME_MSEC  int64 = EB_EXPIRE_TIME_MAX >> 2
)

const (
	rdb6bitLen  = 0
	rdb14bitLen = 1
	rdb32bitLen = 0x80
	rdb64bitLen = 0x81
	rdbEncVal   = 3
	rdbLenErr   = math.MaxUint64

	kFlagMeta  = 243 // redis for RDB_OPCODE_KEY_META Key metadata (module metadata classes, RDB 13)，valkey9.0 for RDB_OPCODE_SLOT_IMPORT 243
	//rdb v1.0.5 add for redis7
	kFlagSlotInfo  = 244 // (Redis 7.4) RDB_OPCODE_SLOT_INFO: slot info
	kFlagFunction2 = 245 // RDB_OPCODE_FUNCTION2: function library data
	kFlagFunction  = 246 // RDB_OPCODE_FUNCTION_PRE_GA: old function library data for 7.0 rc1 and rc2

	// rdb v1.0.0 for redis6
	rdbOpCodeModuleAux = 247 // RDB_OPCODE_MODULE_AUX: Module auxiliary data.
	rdbOpCodeIdle      = 248 // RDB_OPCODE_IDLE: LRU idle time.
	rdbOpCodeFreq      = 249 // RDB_OPCODE_FREQ: LFU frequency.
	rdbOpCodeAux       = 250 // RDB_OPCODE_AUX: RDB aux field.
	rdbOpCodeResizeDB  = 251 // RDB_OPCODE_RESIZEDB: Hash table resize hint.
	rdbOpCodeExpiryMS  = 252 // RDB_OPCODE_EXPIRETIME_MS: Expire time in milliseconds.
	rdbOpCodeExpiry    = 253 // RDB_OPCODE_EXPIRETIME: Old expire time in seconds.
	rdbOpCodeSelectDB  = 254 // RDB_OPCODE_SELECTDB: DB number of the following keys.
	rdbOpCodeEOF       = 255 // RDB_OPCODE_EOF: End of the RDB file.

	//rdb v1.0.5 add for redis7
	moduleTypeNameCharSet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

	rdbModuleOpCodeEOF    = 0 // RDB_MODULE_OPCODE_EOF: End of module value.
	rdbModuleOpCodeSint   = 1 // RDB_MODULE_OPCODE_SINT: Signed integer.
	rdbModuleOpCodeUint   = 2 // RDB_MODULE_OPCODE_UINT: Unsigned integer.
	rdbModuleOpCodeFloat  = 3 // RDB_MODULE_OPCODE_FLOAT: Float.
	rdbModuleOpCodeDouble = 4 // RDB_MODULE_OPCODE_DOUBLE: Double.
	rdbModuleOpCodeString = 5 // RDB_MODULE_OPCODE_STRING: String.

	rdbLoadNone  = 0
	rdbLoadEnc   = (1 << 0)
	rdbLoadPlain = (1 << 1)
	rdbLoadSds   = (1 << 2)

	rdbSaveNode        = 0
	rdbSaveAofPreamble = (1 << 0)

	rdbEncInt8  = 0
	rdbEncInt16 = 1
	rdbEncInt32 = 2
	rdbEncLZF   = 3

	rdbZiplist6bitlenString  = 0
	rdbZiplist14bitlenString = 1
	rdbZiplist32bitlenString = 2

	rdbZiplistInt16 = 0xc0
	rdbZiplistInt32 = 0xd0
	rdbZiplistInt64 = 0xe0
	rdbZiplistInt24 = 0xf0
	rdbZiplistInt8  = 0xfe
	rdbZiplistInt4  = 15

	rdbLpHdrSize           = 6
	rdbLpHdrNumeleUnknown  = math.MaxUint16
	rdbLpMaxIntEncodingLen = 0
	rdbLpMaxBacklenSize    = 5
	rdbLpMaxEntryBacklen   = 34359738367
	rdbLpEncodingInt       = 0
	rdbLpEncodingString    = 1

	rdbLpEncoding7BitUint     = 0
	rdbLpEncoding7BitUintMask = 0x80

	rdbLpEncoding6BitStr     = 0x80
	rdbLpEncoding6BitStrMask = 0xC0

	rdbLpEncoding13BitInt     = 0xC0
	rdbLpEncoding13BitIntMask = 0xE0

	rdbLpEncoding12BitStr     = 0xE0
	rdbLpEncoding12BitStrMask = 0xF0

	rdbLpEncoding16BitInt     = 0xF1
	rdbLpEncoding16BitIntMask = 0xFF

	rdbLpEncoding24BitInt     = 0xF2
	rdbLpEncoding24BitIntMask = 0xFF

	rdbLpEncoding32BitInt     = 0xF3
	rdbLpEncoding32BitIntMask = 0xFF

	rdbLpEncoding64BitInt     = 0xF4
	rdbLpEncoding64BitIntMask = 0xFF

	rdbLpEncoding32BitStr     = 0xF0
	rdbLpEncoding32BitStrMask = 0xFF

	rdbLpEOF = 0xFF

	// rdb v1.0.8 add for redis7
	rdbStream2Version = 1
	rdbStream3Version = 2
)

// rdb v1.0.5 add for redis7. quicklist node container formats
const (
	quickListNodeContainerPlain  = 1 // QUICKLIST_NODE_CONTAINER_PLAIN
	quickListNodeContainerPacked = 2 // QUICKLIST_NODE_CONTAINER_PACKED
)

// 解码，得到objType，根据objType来执行相应的解码动作
// 读取key和属性d.readObject(key, ValueType(objType), expiry)
func (d *decode) decode() error {
	err := d.checkHeader()
	if err != nil {
		return errors.Trace(err)
	}
	d.event.StartRDB(d.rdbVersion)
	var db uint64
	var expiry int64
	//var lruClock int64
	firstDB := true
	for {
		objType, err := d.r.ReadByte()
		if err != nil {
			return errors.Wrap(err, errors.New("read failed"))
		}
		switch objType {
		case kFlagMeta:
			 // 243:{Valkey:SlotImport, Redis 8.0+:RDB_OPCODE_KEY_META}
			if d.isValkey {
				// Valkey 9+: slot import state
				job, err := d.readString()
				if err != nil {
					return errors.Trace(err)
				}
				numSlotRanges, _, err := d.readLength()
				if err != nil {
					return errors.Trace(err)
				}
				var slotFrom, slotTo uint64
				ranges := make([]string, numSlotRanges)
				for i := uint64(0); i < numSlotRanges; i++ {
					slotFrom, _, err = d.readLength()
					if err == nil {
						slotTo, _, err = d.readLength()
					}
					if err != nil {
						return errors.Trace(err)
					}
					ranges[i] = fmt.Sprintf("%d-%d", slotFrom, slotTo)
				}
				_, _ = job, ranges // safe to skip
			} else {
				// return fmt.Errorf("unsupported opcode: RDB_OPCODE_KEY_META (243) in Redis RDB version %d", d.rdbVersion)
				/* RDB_OPCODE_KEY_META (RDB 13，opcode value 243): 该操作码用于在键值对前附加元数据.*/
				// 当一个模块（比如 RedisJSON、RediSearch）创建了自己的数据类型时，它可能需要将一些额外的、非标准的元数据（如索引信息、字段定义等）和键值一起持久化
				/* With metadata, type = RDB_OPCODE_KEY_META. Layout: [<META>,]<TYPE>,<KEY>,<VALUE> */
				//大致排列：[RDB_OPCODE_KEY_META] [numClasses] [Class1_ID] [Class1_Data] [Class2_ID] [Class2_Data] ... [实际对象类型] [Key] [Value]

				// 1. 跳过元数据
				if err = d.skipKeyMeta(); err != nil {
					return err
				}
				// 2. 读取实际对象类型
				actualTypeByte, err := d.r.ReadByte()
				if err != nil {
					return err
				}
				// _ = actualTypeByte
				actualType := int(actualTypeByte)
				// 4. 读取 key
				key, err := d.readString()
				if err != nil {
					return errors.Trace(err)
				}
				// 5. 读取 value（使用 actualType 和当前的 expiry）
				if err := d.readObject(key, ValueType(actualType), expiry); err != nil {
					return errors.Trace(err)
				}
				// 6. 重置辅助状态
				expiry = 0
				d.lruIdle = 0
				d.lfuFreq = 0
				/* Read next opcode. */
			}
		case kFlagSlotInfo:
			_, _, _ = d.readLength() // slot_id
			_, _, _ = d.readLength() // slot_size
			_, _, _ = d.readLength() // expires_slot_size
		case kFlagFunction:
			log.Panicf("function library data not supported, need PR to support")
		case kFlagFunction2:
			function, err := d.readString()
			if err != nil {
				return errors.Trace(err)
			}
			log.Debugf("function: %s", function)
			//e := entry.NewEntry()
			//e.Argv = []string{"function", "load", function}
			//ld.ch <- e
		case rdbOpCodeFreq:
			b, err := d.r.ReadByte()
			d.lfuFreq = int(b)
			if err != nil {
				return errors.Trace(err)
			}
		case rdbOpCodeIdle:
			idle, _, err := d.readLength()
			if err != nil {
				return errors.Trace(err)
			}
			d.lruIdle = uint64(idle)
		case rdbOpCodeAux:
			auxKey, err := d.readString()
			if err != nil {
				return errors.Trace(err)
			}
			auxVal, err := d.readString()
			if err != nil {
				return errors.Trace(err)
			}
			d.event.Aux(auxKey, auxVal)
		case rdbOpCodeResizeDB:
			dbSize, _, err := d.readLength()
			if err != nil {
				return errors.Trace(err)
			}
			expiresSize, _, err := d.readLength()
			if err != nil {
				return errors.Trace(err)
			}
			d.event.ResizeDatabase(uint32(dbSize), uint32(expiresSize))
		case rdbOpCodeExpiryMS:
			_, err := io.ReadFull(d.r, d.intBuf)
			if err != nil {
				return errors.Trace(err)
			}
			expiry = int64(binary.LittleEndian.Uint64(d.intBuf))
		case rdbOpCodeExpiry:
			_, err := io.ReadFull(d.r, d.intBuf[:4])
			if err != nil {
				return errors.Trace(err)
			}
			expiry = int64(binary.LittleEndian.Uint32(d.intBuf)) * 1000
		case rdbOpCodeSelectDB:
			if !firstDB {
				d.event.EndDatabase(int(db))
			}
			db, _, err = d.readLength()
			if err != nil {
				return errors.Trace(err)
			}
			d.event.StartDatabase(int(db))
		case rdbOpCodeEOF:
			d.event.EndDatabase(int(db))
			d.event.EndRDB()
			return nil
		case rdbOpCodeModuleAux:
			/* AUX: Auxiliary data for modules. */
			// skip module
			moduleId := structure.ReadLength(d.r) // module id
			moduleName := types.ModuleTypeNameByID(moduleId)
			log.Debugf("RDB module aux: module_id=[%d], module_name=[%s]", moduleId, moduleName)
			_ = structure.ReadLength(d.r) // when_opcode
			_ = structure.ReadLength(d.r) // when
			opcode := structure.ReadLength(d.r)
			for opcode != rdbModuleOpCodeEOF {
				switch opcode {
				case rdbModuleOpCodeSint, rdbModuleOpCodeUint:
					_ = structure.ReadLength(d.r)
				case rdbModuleOpCodeFloat:
					_ = structure.ReadFloat(d.r)
				case rdbModuleOpCodeDouble:
					_ = structure.ReadDouble(d.r)
				case rdbModuleOpCodeString:
					_ = structure.ReadString(d.r)
				default:
					log.Panicf("module aux opcode not found. module_name=[%s], opcode=[%d]", moduleName, opcode)
				}
				// 读取下一个 opcode
				opcode = structure.ReadLength(d.r)
			}
		default:
			/* Read key */
			key, err := d.readString()
			if err != nil {
				return errors.Trace(err)
			}

			 /* Read value */
			err= d.readObject(key, ValueType(objType), expiry)
			if err != nil {
				return errors.Trace(err)
			}
			expiry = 0
			d.lruIdle = 0
			d.lfuFreq = 0
		}
	}

}

// 读取redisObject,RedisObject is interface for a redis object
func (d *decode) readObject(key []byte, typ ValueType, expiry int64) error {
	d.info = &Info{
		Idle: d.lruIdle,
		Freq: d.lfuFreq,
	}

	// 调试
	// if string(key) == "xxx" {
    //     fmt.Printf("object type %d for key %s\n", typ, string(key))
	// }

	switch typ {
	case TypeString:
		value, err := d.readString()
		if err != nil {
			return errors.Trace(err)
		}
		d.info.Encoding = "string"
		d.event.Set(key, value, expiry, d.info)
	case TypeList:
		length, _, err := d.readLength()
		if err != nil {
			return errors.Trace(err)
		}
		d.info.Encoding = "linkedlist"
		d.event.StartList(key, int64(length), expiry, d.info)
		for length > 0 {
			length--
			value, err := d.readString()
			if err != nil {
				return errors.Trace(err)
			}
			d.event.Rpush(key, value, 0) //qucklist2中节点有两种编码1和2，其他数据类型传0
		}
		d.event.EndList(key)
	case TypeListQuickList:
		length, _, err := d.readLength()
		if err != nil {
			return errors.Trace(err)
		}
		d.info.Encoding = "quicklist"
		d.info.Zips = length
		d.event.StartList(key, int64(-1), expiry, d.info)
		for length > 0 {
			length--
			d.readZiplist(key, 0, false)
		}
		d.event.EndList(key)
	case TypeSet:
		cardinality, _, err := d.readLength()
		if err != nil {
			return errors.Trace(err)
		}
		d.info.Encoding = "hashtable"
		d.event.StartSet(key, int64(cardinality), expiry, d.info)
		for cardinality > 0 {
			cardinality--
			member, err := d.readString()
			if err != nil {
				return errors.Trace(err)
			}
			d.event.Sadd(key, member)
		}
		d.event.EndSet(key)
	case TypeZSet2:
		fallthrough
	case TypeZSet:
		cardinality, _, err := d.readLength()
		if err != nil {
			return errors.Trace(err)
		}
		d.info.Encoding = "skiplist"
		d.event.StartZSet(key, int64(cardinality), expiry, d.info)
		for cardinality > 0 {
			cardinality--
			member, err := d.readString()
			if err != nil {
				return errors.Trace(err)
			}
			var score float64
			if typ == TypeZSet2 {
				score, err = d.readBinaryFloat64()
				if err != nil {
					return errors.Trace(err)
				}
			} else {
				score, err = d.readFloat64()
				if err != nil {
					return errors.Trace(err)
				}
			}
			d.event.Zadd(key, score, member)
		}
		d.event.EndZSet(key)
	case TypeHash:
		length, _, err := d.readLength()
		if err != nil {
			return errors.Trace(err)
		}
		d.info.Encoding = "hashtable"
		d.event.StartHash(key, int64(length), expiry, d.info)
		for length > 0 {
			length--
			field, err := d.readString()
			if err != nil {
				return errors.Trace(err)
			}
			value, err := d.readString()
			if err != nil {
				return errors.Trace(err)
			}
			d.event.Hset(key, field, value)
		}
		d.event.EndHash(key)
	case TypeHashZipMap:
		return errors.Trace(d.readZipmap(key, expiry))
	case TypeListZipList:
		return errors.Trace(d.readZiplist(key, expiry, true))
	case TypeSetIntSet:
		return errors.Trace(d.readIntset(key, expiry))
	case TypeZSetZipList:
		return errors.Trace(d.readZiplistZset(key, expiry))
	case TypeHashZipList:
		return errors.Trace(d.readZiplistHash(key, expiry))
	case TypeStreamListPacks:
		return errors.Trace(d.readStream(key, expiry))
	case TypeModule:
		fallthrough
	case TypeModule2:
		return d.readModule(key, expiry)
	//TypeListQuickList2、TypeHashListPack、TypeZsetListPack、TypeSetListPack 参考的阿里云长期维护的RedisShake
	case TypeListQuickList2:
		return errors.Trace(d.readQuickList2(key, expiry))
	case TypeHashListPack:
		list, buf := structure.ReadListPack2(d.r)
		size := len(list)
		d.info.Encoding = "listpack"
		d.info.SizeOfValue = int(buf)
		d.event.StartHash(key, int64(size/2), expiry, d.info)
		for i := 0; i < size; i += 2 {
			fieldStr := list[i]
			valueStr := list[i+1]
			FiledBytes := []byte(fieldStr)
			ValueBytes := []byte(valueStr)
			d.event.Hset(key, FiledBytes, ValueBytes)
		}
		d.event.EndHash(key)
	case TypeZSetListPack:
		list, buf := structure.ReadListPack2(d.r)
		size := len(list)
		if size%2 != 0 {
			log.Panicf("zset listpack size is not even. size=[%d]", size)
		}
		d.info.Encoding = "listpack"
		d.info.SizeOfValue = int(buf)
		d.event.StartZSet(key, int64(size/2), expiry, d.info)
		for i := 0; i < size; i += 2 {
			memberStr := list[i]
			scoreStr := list[i+1]
			memberBytes := []byte(memberStr)
			scoreFloat, _ := strconv.ParseFloat(scoreStr, 64)
			d.event.Zadd(key, scoreFloat, memberBytes)
		}
		d.event.EndZSet(key)
	case TypeSetListPack:
		elements, buf := structure.ReadListPack2(d.r)
		size := len(elements)
		d.info.Encoding = "listpack"
		d.info.SizeOfValue = int(buf)
		d.event.StartSet(key, int64(size), expiry, d.info)
		for _, eleStr := range elements {
			elerBytes := []byte(eleStr)
			d.event.Sadd(key, elerBytes)
		}
		d.event.EndSet(key)
	case TypeStreamListPacks2:
		return errors.Trace(d.readStreamListPacks(rdbStream2Version,key, expiry))
	case TypeStreamListPacks3:
		return errors.Trace(d.readStreamListPacks(rdbStream3Version,key, expiry))
	case TypeHashMetaDataPreGa22:
		// Type 22: Redis uses RDB_TYPE_HASH_METADATA_PRE_GA, Valkey uses RDB_TYPE_HASH_2
		if d.isValkey {
			// Valkey 9+ Hash2: absolute timestamps after each field-value pair 过期时间戳在value的后面
			return errors.Trace(d.readHashExValkey(key, expiry)) // Valkey 9.0 HASH_2 format: field, value, TTL (8-byte ms)
		} else {
			return errors.Trace(d.readHashTtl(key, expiry, true)) // Redis 8.0 format: TTL (length), field, value
		}
	case TypeHashListPackExPre23:
		return errors.Trace(d.readHashListPackTtl(key, expiry, true))
	case TypeHashMetaData24:
		return errors.Trace(d.readHashTtl(key, expiry, false))
	case TypeHashListPackEx25:
		return errors.Trace(d.readHashListPackTtl(key, expiry, false))
	default:
		return fmt.Errorf("rdb: unknown object type %d for key %s", typ, key)
	}
	return nil
}

func (d *decode) readModule(key []byte, expiry int64) error {
    moduleid, _, err := d.readLength()
    if err != nil {
        return errors.Trace(err)
    }
    ModuleTypeName := types.ModuleTypeNameByID(moduleid)

    // 关键：跳过整个模块的辅助数据（Aux Data），因为我这来是跳过模块解析，因此不关心模块的版本号了
    if err := structure.SkipModuleAuxData(d.r); err != nil {
        return fmt.Errorf("skip module aux data failed for key %s, type %s: %v", key, ModuleTypeName, err)
    }

    // 如果想记录日志（可选），在这里打印：
    // log.Warnf("Skipped module data for key %s, type: %s", key, ModuleTypeName)

    // 返回 nil，表示成功跳过，继续解析下一个键
    return nil
}

func (d *decode) readStream(key []byte, expiry int64) error {
	cardinality, _, err := d.readLength()
	if err != nil {
		return errors.Trace(err)
	}
	d.info.Encoding = "listpack"
	d.event.StartStream(key, int64(cardinality), expiry, d.info)
	for cardinality > 0 {
		cardinality--

		streamID, err := d.readString()
		if err != nil {
			return errors.Trace(err)
		}
		/*
		   IDms := strconv.FormatUint(binary.BigEndian.Uint64(streamID[:8]), 10)
		   IDseq := strconv.FormatUint(binary.BigEndian.Uint64(streamID[8:]), 10)
		   fmt.Println(string(key))
		   fmt.Println(IDms + "-" + IDseq)
		*/
		listPack, err := d.readString()
		if err != nil {
			return errors.Trace(err)
		}
		d.event.Xadd(key, streamID, listPack)
	}
	var items, lastIDms, lastIDseq uint64
	items, _, err = d.readLength()
	if err != nil {
		return errors.Trace(err)
	}
	lastIDms, _, err = d.readLength()
	if err != nil {
		return errors.Trace(err)
	}
	lastIDseq, _, err = d.readLength()
	if err != nil {
		return errors.Trace(err)
	}

	lastEntryID := fmt.Sprintf("%d-%d", lastIDms, lastIDseq)

	//TODO output consumer groups
	var groupsCount uint64
	groupsCount, _, err = d.readLength()
	if err != nil {
		return errors.Trace(err)
	}

	cgroupsData := make(StreamGroups, 0, groupsCount)
	for groupsCount > 0 {
		groupsCount--

		cgname, err := d.readString()
		if err != nil {
			return errors.Trace(err)
		}
		gIDms, _, err := d.readLength()
		if err != nil {
			return errors.Trace(err)
		}
		gIDseq, _, err := d.readLength()
		if err != nil {
			return errors.Trace(err)
		}

		lastCgEntryID := fmt.Sprintf("%d-%d", gIDms, gIDseq)

		pelSize, _, err := d.readLength()
		if err != nil {
			return errors.Trace(err)
		}

		groupPendingEntries := make([]*StreamPendingEntry, 0, pelSize)
		for pelSize > 0 {
			pelSize--
			// d.readUint64()
			ms,err:=d.readUint64()
			if err != nil {
				return errors.Trace(err)
			}
			seq,err := d.readUint64()
			if err !=nil {
				return errors.Trace(err)
			}
			streamID :=&StreamId{
				Ms: ms,
				Sequence: seq,
			}

			deliveryTime, err := d.readUint64()
			if err != nil {
				return errors.Trace(err)
			}
			deliveryCount, _, err := d.readLength()
			if err != nil {
				return errors.Trace(err)
			}

			groupPendingEntries = append(groupPendingEntries, &StreamPendingEntry{
				ID:            streamID,
				DeliveryTime:  deliveryTime,
				DeliveryCount: deliveryCount,
			})
		}

		consumersNum, _, err := d.readLength()
		if err != nil {
			return errors.Trace(err)
		}

		consumersData := make([]*StreamConsumerData, 0, consumersNum)
		for consumersNum > 0 {
			consumersNum--
			cname, err := d.readString()
			if err != nil {
				return errors.Trace(err)
			}
			seenTime, err := d.readUint64()
			if err != nil {
				return errors.Trace(err)
			}
			pelSize, _, err := d.readLength()
			if err != nil {
				return errors.Trace(err)
			}
			consumerPendingEntries := make([]*StreamConsumerPendingEntry, 0, pelSize)
			for pelSize > 0 {
				pelSize--
				rawid := make([]byte, 16)
				n, err := io.ReadFull(d.r, rawid)
				if err != nil {
					return errors.Trace(err)
				}
				if n != 16 {
					return errors.Errorf("expected %d got %d", 16, n)
				}

				consumerPendingEntries = append(consumerPendingEntries, &StreamConsumerPendingEntry{ID: rawid})
			}

			consumersData = append(consumersData, &StreamConsumerData{
				Name:     cname,
				SeenTime: seenTime,
				Pending:  consumerPendingEntries,
			})
		}

		cgroupsData = append(cgroupsData, &StreamGroup{
			Name:        cgname,
			LastEntryId: lastCgEntryID,
			Pending:     groupPendingEntries,
			Consumers:   consumersData,
		})
	}

	d.event.EndStream(key, items, lastEntryID, cgroupsData)
	return nil
}

func (d *decode) readZipmap(key []byte, expiry int64) error {
	var length int
	zipmap, err := d.readString()
	if err != nil {
		return errors.Trace(err)
	}
	buf := newSliceBuffer(zipmap)
	lenByte, err := buf.ReadByte()
	if err != nil {
		return errors.Trace(err)
	}
	if lenByte >= 254 { // we need to count the items manually
		length, err = countZipmapItems(buf)
		length /= 2
		if err != nil {
			return errors.Trace(err)
		}
	} else {
		length = int(lenByte)
	}
	d.info.Encoding = "zipmap"
	d.info.SizeOfValue = len(zipmap)
	d.event.StartHash(key, int64(length), expiry, d.info)
	for i := 0; i < length; i++ {
		field, err := readZipmapItem(buf, false)
		if err != nil {
			return errors.Trace(err)
		}
		value, err := readZipmapItem(buf, true)
		if err != nil {
			return errors.Trace(err)
		}
		d.event.Hset(key, field, value)
	}
	d.event.EndHash(key)
	return nil
}

func readZipmapItem(buf *sliceBuffer, readFree bool) ([]byte, error) {
	length, free, err := readZipmapItemLength(buf, readFree)
	if err != nil {
		return nil, err
	}
	if length == -1 {
		return nil, nil
	}
	value, err := buf.Slice(length)
	if err != nil {
		return nil, err
	}
	_, err = buf.Seek(int64(free), 1)
	return value, err
}

func countZipmapItems(buf *sliceBuffer) (int, error) {
	n := 0
	for {
		strLen, free, err := readZipmapItemLength(buf, n%2 != 0)
		if err != nil {
			return 0, err
		}
		if strLen == -1 {
			break
		}
		_, err = buf.Seek(int64(strLen)+int64(free), 1)
		if err != nil {
			return 0, err
		}
		n++
	}
	_, err := buf.Seek(0, 0)
	return n, err
}

func readZipmapItemLength(buf *sliceBuffer, readFree bool) (int, int, error) {
	b, err := buf.ReadByte()
	if err != nil {
		return 0, 0, err
	}
	switch b {
	case 253:
		s, err := buf.Slice(5)
		if err != nil {
			return 0, 0, err
		}
		return int(binary.BigEndian.Uint32(s)), int(s[4]), nil
	case 254:
		return 0, 0, fmt.Errorf("rdb: invalid zipmap item length")
	case 255:
		return -1, 0, nil
	}
	var free byte
	if readFree {
		free, err = buf.ReadByte()
	}
	return int(b), int(free), err
}

func (d *decode) readListPack() error {
	listpack, err := d.readString()
	//fmt.Println(len(listpack))
	//fmt.Println(listpack)
	if err != nil {
		return errors.Trace(err)
	}
	buf := newSliceBuffer(listpack)
	buf.Slice(4) // total bytes
	numElements, _ := buf.Slice(2)
	num := int64(binary.LittleEndian.Uint16(numElements))
	if err != nil {
		return errors.Trace(err)
	}
	for {
		num--
		b, _ := buf.Slice(1)
		if b[0] == byte(rdbLpEOF) {
			fmt.Println("eof")
			break
		}
		//lpGet(b, buf)
	}
	return nil
}

func lpGet(b []byte, buf *sliceBuffer) {
	var val int64
	var uval, negstart, negmax uint64
	fmt.Println(b[0], lpEncodingIs7BitUint(b[0]))
	if lpEncodingIs7BitUint(b[0]) {
		fmt.Println("lpEncodingIs7BitUint")
		negstart = math.MaxUint64
		negmax = 0
		uval = uint64(b[0] & 0x7F)
	} else if lpEncodingIs6BitStr(b[0]) {
		fmt.Println("lpEncodingIs6BitStr")
		len := lpEncoding6BitStrLen(b)
		str, _ := buf.Slice(int(len))
		fmt.Print(string(str))
	} else if lpEncodingIs13BitInt(b[0]) {
		fmt.Println("lpEncodingIs13BitInt")
		tmp, _ := buf.Slice(1)
		b = append(b, tmp...)
		uval = (uint64(b[0]&0x1f) << 8) | uint64(b[1])
		negstart = uint64(1) << 12
		negmax = 8191
	} else if lpEncodingIs16BitInt(b[0]) {
		fmt.Println("lpEncodingIs16BitInt")
		tmp, _ := buf.Slice(2)
		b = append(b, tmp...)
		uval = uint64(b[1]) |
			uint64(b[2])<<8
		negstart = uint64(1) << 15
		negmax = math.MaxUint16
	} else if lpEncodingIs24BitInt(b[0]) {
		fmt.Println("lpEncodingIs24BitInt")
		tmp, _ := buf.Slice(3)
		b = append(b, tmp...)
		uval = uint64(b[1]) |
			uint64(b[2])<<8 |
			uint64(b[3])<<16
		negstart = uint64(1) << 23
		negmax = math.MaxUint32 >> 8
	} else if lpEncodingIs32BitInt(b[0]) {
		fmt.Println("lpEncodingIs32BitInt")
		tmp, _ := buf.Slice(4)
		b = append(b, tmp...)
		uval = uint64(b[1]) |
			uint64(b[2])<<8 |
			uint64(b[3])<<16 |
			uint64(b[4])<<24
		negstart = uint64(1) << 31
		negmax = math.MaxUint32
	} else if lpEncodingIs64BitInt(b[0]) {
		fmt.Println("lpEncodingIs64BitInt")
		tmp, _ := buf.Slice(8)
		b = append(b, tmp...)
		uval = uint64(b[1]) |
			uint64(b[2])<<8 |
			uint64(b[3])<<16 |
			uint64(b[4])<<24 |
			uint64(b[5])<<32 |
			uint64(b[6])<<40 |
			uint64(b[7])<<48 |
			uint64(b[8])<<56
		negstart = uint64(1) << 63
		negmax = math.MaxUint64
	} else if lpEncodingIs12BitStr(b[0]) {
		fmt.Println("lpEncodingIs12BitStr")
		tmp, _ := buf.Slice(1)
		b = append(b, tmp...)
		len := lpEncoding12BitStrLen(b)
		fmt.Println(len)
		str, _ := buf.Slice(int(len))
		fmt.Print(string(str))
	} else if lpEncodingIs32BitStr(b[0]) {
		fmt.Println("lpEncodingIs32BitStr")
		tmp, _ := buf.Slice(4)
		b = append(b, tmp...)
		len := lpEncoding32BitStrLen(b)
		str, _ := buf.Slice(int(len))
		fmt.Print(string(str))
	} else {
		fmt.Println("else")
		uval = uint64(12345678900000000) + uint64(b[0])
		negstart = math.MaxUint64
		negmax = 0
	}

	if uval >= negstart {
		uval = negmax - uval
		val = int64(uval)
		val = -val - 1
	} else {
		val = int64(uval)
	}

	fmt.Printf("%d\n", val)
}

func lpEncodingIs7BitUint(b byte) bool {
	return (((b) & rdbLpEncoding7BitUintMask) == rdbLpEncoding7BitUint)
}
func lpEncodingIs6BitStr(b byte) bool {
	return (((b) & rdbLpEncoding6BitStrMask) == rdbLpEncoding6BitStr)
}
func lpEncodingIs13BitInt(b byte) bool {
	return (((b) & rdbLpEncoding13BitIntMask) == rdbLpEncoding13BitInt)
}
func lpEncodingIs12BitStr(b byte) bool {
	return (((b) & rdbLpEncoding12BitStrMask) == rdbLpEncoding12BitStr)
}
func lpEncodingIs16BitInt(b byte) bool {
	return (((b) & rdbLpEncoding16BitIntMask) == rdbLpEncoding16BitInt)
}
func lpEncodingIs24BitInt(b byte) bool {
	return (((b) & rdbLpEncoding24BitIntMask) == rdbLpEncoding24BitInt)
}
func lpEncodingIs32BitInt(b byte) bool {
	return (((b) & rdbLpEncoding32BitIntMask) == rdbLpEncoding32BitInt)
}
func lpEncodingIs64BitInt(b byte) bool {
	return (((b) & rdbLpEncoding64BitIntMask) == rdbLpEncoding64BitInt)
}
func lpEncodingIs32BitStr(b byte) bool {
	return (((b) & rdbLpEncoding32BitStrMask) == rdbLpEncoding32BitStr)
}
func lpEncoding6BitStrLen(b []byte) uint32 {
	return uint32(b[0] & 0x3F)
}
func lpEncoding12BitStrLen(b []byte) uint32 {
	return (uint32((b)[0]&0xF) << 8) | uint32((b)[1])
}
func lpEncoding32BitStrLen(b []byte) uint32 {
	return (uint32(b[1]) << 0) |
		(uint32(b[2]) << 8) |
		(uint32(b[3]) << 16) |
		(uint32(b[4]) << 24)
}

func (d *decode) readZiplist(key []byte, expiry int64, addListEvents bool) error {
	ziplist, err := d.readString()
	if err != nil {
		return errors.Trace(err)
	}
	buf := newSliceBuffer(ziplist)
	length, err := readZiplistLength(buf)
	if err != nil {
		return errors.Trace(err)
	}
	if addListEvents {
		d.info.Encoding = "ziplist"
		d.info.SizeOfValue = len(ziplist)
		d.event.StartList(key, length, expiry, d.info)
	}
	for i := int64(0); i < length; i++ {
		entry, err := readZiplistEntry(buf)
		if err != nil {
			return errors.Trace(err)
		}
		d.event.Rpush(key, entry,0)
	}
	if addListEvents {
		d.event.EndList(key)
	}
	return nil
}

func (d *decode) readZiplistZset(key []byte, expiry int64) error {
	ziplist, err := d.readString()
	if err != nil {
		return errors.Trace(err)
	}
	buf := newSliceBuffer(ziplist)
	cardinality, err := readZiplistLength(buf)
	if err != nil {
		return errors.Trace(err)
	}
	cardinality /= 2
	d.info.Encoding = "ziplist"
	d.info.SizeOfValue = len(ziplist)
	d.event.StartZSet(key, cardinality, expiry, d.info)
	for i := int64(0); i < cardinality; i++ {
		member, err := readZiplistEntry(buf)
		if err != nil {
			return errors.Trace(err)
		}
		scoreBytes, err := readZiplistEntry(buf)
		if err != nil {
			return errors.Trace(err)
		}
		score, err := strconv.ParseFloat(string(scoreBytes), 64)
		if err != nil {
			return errors.Trace(err)
		}
		d.event.Zadd(key, score, member)
	}
	d.event.EndZSet(key)
	return nil
}

func (d *decode) readZiplistHash(key []byte, expiry int64) error {
	ziplist, err := d.readString()
	if err != nil {
		return errors.Trace(err)
	}
	buf := newSliceBuffer(ziplist)
	length, err := readZiplistLength(buf)
	if err != nil {
		return errors.Trace(err)
	}
	length /= 2
	d.info.Encoding = "ziplist"
	d.info.SizeOfValue = len(ziplist)
	d.event.StartHash(key, length, expiry, d.info)
	for i := int64(0); i < length; i++ {
		field, err := readZiplistEntry(buf)
		if err != nil {
			return errors.Trace(err)
		}
		value, err := readZiplistEntry(buf)
		if err != nil {
			return errors.Trace(err)
		}
		d.event.Hset(key, field, value)
	}
	d.event.EndHash(key)
	return nil
}

func readZiplistLength(buf *sliceBuffer) (int64, error) {
	buf.Seek(8, 0) // skip the zlbytes and zltail
	lenBytes, err := buf.Slice(2)
	if err != nil {
		return 0, err
	}
	return int64(binary.LittleEndian.Uint16(lenBytes)), nil
}

func readZiplistEntry(buf *sliceBuffer) ([]byte, error) {
	prevLen, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}
	if prevLen == 254 {
		buf.Seek(4, 1) // skip the 4-byte prevlen
	}

	header, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}
	switch {
	case header>>6 == rdbZiplist6bitlenString:
		return buf.Slice(int(header & 0x3f))
	case header>>6 == rdbZiplist14bitlenString:
		b, err := buf.ReadByte()
		if err != nil {
			return nil, err
		}
		return buf.Slice((int(header&0x3f) << 8) | int(b))
	case header>>6 == rdbZiplist32bitlenString:
		lenBytes, err := buf.Slice(4)
		if err != nil {
			return nil, err
		}
		return buf.Slice(int(binary.BigEndian.Uint32(lenBytes)))
	case header == rdbZiplistInt16:
		intBytes, err := buf.Slice(2)
		if err != nil {
			return nil, err
		}
		return []byte(strconv.FormatInt(int64(int16(binary.LittleEndian.Uint16(intBytes))), 10)), nil
	case header == rdbZiplistInt32:
		intBytes, err := buf.Slice(4)
		if err != nil {
			return nil, err
		}
		return []byte(strconv.FormatInt(int64(int32(binary.LittleEndian.Uint32(intBytes))), 10)), nil
	case header == rdbZiplistInt64:
		intBytes, err := buf.Slice(8)
		if err != nil {
			return nil, err
		}
		return []byte(strconv.FormatInt(int64(binary.LittleEndian.Uint64(intBytes)), 10)), nil
	case header == rdbZiplistInt24:
		intBytes := make([]byte, 4)
		_, err := buf.Read(intBytes[1:])
		if err != nil {
			return nil, err
		}
		return []byte(strconv.FormatInt(int64(int32(binary.LittleEndian.Uint32(intBytes))>>8), 10)), nil
	case header == rdbZiplistInt8:
		b, err := buf.ReadByte()
		return []byte(strconv.FormatInt(int64(int8(b)), 10)), err
	case header>>4 == rdbZiplistInt4:
		return []byte(strconv.FormatInt(int64(header&0x0f)-1, 10)), nil
	}

	return nil, fmt.Errorf("rdb: unknown ziplist header byte: %d", header)
}

func (d *decode) readIntset(key []byte, expiry int64) error {
	intset, err := d.readString()
	if err != nil {
		return errors.Trace(err)
	}
	buf := newSliceBuffer(intset)
	intSizeBytes, err := buf.Slice(4)
	if err != nil {
		return errors.Trace(err)
	}
	intSize := binary.LittleEndian.Uint32(intSizeBytes)

	if intSize != 2 && intSize != 4 && intSize != 8 {
		return fmt.Errorf("rdb: unknown intset encoding: %d", intSize)
	}

	lenBytes, err := buf.Slice(4)
	if err != nil {
		return errors.Trace(err)
	}
	cardinality := binary.LittleEndian.Uint32(lenBytes)

	d.info.SizeOfValue = len(intset)
	d.info.Encoding = "intset"
	d.event.StartSet(key, int64(cardinality), expiry, d.info)
	for i := uint32(0); i < cardinality; i++ {
		intBytes, err := buf.Slice(int(intSize))
		if err != nil {
			return errors.Trace(err)
		}
		var intString string
		switch intSize {
		case 2:
			intString = strconv.FormatInt(int64(int16(binary.LittleEndian.Uint16(intBytes))), 10)
		case 4:
			intString = strconv.FormatInt(int64(int32(binary.LittleEndian.Uint32(intBytes))), 10)
		case 8:
			intString = strconv.FormatInt(int64(int64(binary.LittleEndian.Uint64(intBytes))), 10)
		}
		d.event.Sadd(key, []byte(intString))
	}
	d.event.EndSet(key)
	return nil
}


func (d *decode) checkHeader() error {
	// magic + version
	header := make([]byte, 9)
	_, err := io.ReadFull(d.r, header)
	if err == io.EOF {
		return errors.New("empty file")
	}
	if err != nil {
		return fmt.Errorf("io error: %v", err)
	}

	var version int
	if bytes.Equal(header[:5], []byte("REDIS")) {
		// Redis format: "REDIS" (5 bytes) + version (4 bytes), e.g., "REDIS0012"
		version, err = strconv.Atoi(string(header[5:]))
		d.isValkey = false
	} else if bytes.Equal(header[:6], []byte("VALKEY")) {
		// Valkey 9.0+ format: "VALKEY" (6 bytes) + version (3 bytes), e.g., "VALKEY080"
		version, err = strconv.Atoi(string(header[6:]))
		d.isValkey = true
	} else {
		log.Panicf("verify magic string, invalid file format. bytes=[%v]", header[:6])
	}

	if err != nil {
		log.Panicf("%v", err)
	}

    // if !d.isValkey && (version > maxVersionRedis) {
	// 	return fmt.Errorf("cannot parse version: %d", version)
	// }
	// if d.isvalkey && (version > maxVersionValkey) {
	// 	return fmt.Errorf("cannot parse version: %d", version)
	// }

	d.rdbVersion = int(version)
	return nil

}


func (d *decode) readString() ([]byte, error) {
	length, encoded, err := d.readLength()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if encoded {
		switch length {
		case rdbEncInt8:
			i, err := d.readUint8()
			return []byte(strconv.FormatInt(int64(int8(i)), 10)), errors.Trace(err)
		case rdbEncInt16:
			i, err := d.readUint16()
			return []byte(strconv.FormatInt(int64(int16(i)), 10)), errors.Trace(err)
		case rdbEncInt32:
			i, err := d.readUint32()
			return []byte(strconv.FormatInt(int64(int32(i)), 10)), errors.Trace(err)
		case rdbEncLZF:
			clen, _, err := d.readLength()
			if err != nil {
				return nil, errors.Trace(err)
			}
			ulen, _, err := d.readLength()
			if err != nil {
				return nil, errors.Trace(err)
			}
			compressed := make([]byte, clen)
			_, err = io.ReadFull(d.r, compressed)
			if err != nil {
				return nil, errors.Trace(err)
			}
			decompressed := lzfDecompress(compressed, int(ulen))
			if len(decompressed) != int(ulen) {
				return nil, fmt.Errorf("decompressed string length %d didn't match expected length %d", len(decompressed), ulen)
			}
			return decompressed, nil
		default:
			return nil, errors.Errorf("Unknown RDB string encoding type %d", length)
		}
	}

	if length == rdbLenErr {
		return nil, nil
	}

	str := make([]byte, length)
	_, err = io.ReadFull(d.r, str)
	if err != nil {
		return str, errors.Wrap(err, errors.New("readfailed"))
	}
	return str, nil
}

func (d *decode) readUint8() (uint8, error) {
	b, err := d.r.ReadByte()
	if err != nil {
		return uint8(b), errors.Wrap(err, errors.New("readfailed"))
	}
	return uint8(b), nil
}

func (d *decode) readUint16() (uint16, error) {
	_, err := io.ReadFull(d.r, d.intBuf[:2])
	if err != nil {
		return 0, errors.Wrap(err, errors.New("readfailed"))
	}
	return binary.LittleEndian.Uint16(d.intBuf), nil
}

func (d *decode) readUint32() (uint32, error) {
	_, err := io.ReadFull(d.r, d.intBuf[:4])
	if err != nil {
		return 0, errors.Wrap(err, errors.New("readfailed"))
	}
	return binary.LittleEndian.Uint32(d.intBuf), nil
}

func (d *decode) readUint64() (uint64, error) {
	_, err := io.ReadFull(d.r, d.intBuf)
	if err != nil {
		return 0, errors.Wrap(err, errors.New("readfailed"))
	}
	return binary.LittleEndian.Uint64(d.intBuf), nil
}

func (d *decode) readUint32Big() (uint32, error) {
	_, err := io.ReadFull(d.r, d.intBuf[:4])
	if err != nil {
		return 0, errors.Wrap(err, errors.New("readfailed"))
	}
	return binary.BigEndian.Uint32(d.intBuf), nil
}

func (d *decode) readUint64Big() (uint64, error) {
	_, err := io.ReadFull(d.r, d.intBuf)
	if err != nil {
		return 0, errors.Wrap(err, errors.New("readfailed"))
	}
	return binary.BigEndian.Uint64(d.intBuf), nil
}

func (d *decode) readBinaryFloat64() (float64, error) {
	floatBytes := make([]byte, 8)
	_, err := io.ReadFull(d.r, floatBytes)
	if err != nil {
		return 0, errors.Wrap(err, errors.New("readfailed"))
	}
	return math.Float64frombits(binary.LittleEndian.Uint64(floatBytes)), nil
}

// Doubles are saved as strings prefixed by an unsigned
// 8 bit integer specifying the length of the representation.
// This 8 bit integer has special values in order to specify the following
// conditions:
// 253: not a number
// 254: + inf
// 255: - inf
func (d *decode) readFloat64() (float64, error) {
	length, err := d.readUint8()
	if err != nil {
		return 0, err
	}
	switch length {
	case 253:
		return math.NaN(), nil
	case 254:
		return math.Inf(0), nil
	case 255:
		return math.Inf(-1), nil
	default:
		floatBytes := make([]byte, length)
		_, err := io.ReadFull(d.r, floatBytes)
		if err != nil {
			return 0, err
		}
		f, err := strconv.ParseFloat(string(floatBytes), 64)
		return f, err
	}
}

func (d *decode) readLength() (uint64, bool, error) {
	b, err := d.r.ReadByte()
	if err != nil {
		return 0, false, errors.Wrap(err, errors.New("readfailed"))
	}
	// The first two bits of the first byte are used to indicate the length encoding type
	typ := (b & 0xc0) >> 6
	switch typ {
	case rdb6bitLen:
		// When the first two bits are 00, the next 6 bits are the length.
		return uint64(b & 0x3f), false, nil
	case rdb14bitLen:
		// When the first two bits are 01, the next 14 bits are the length.
		bb, err := d.r.ReadByte()
		if err != nil {
			return 0, false, errors.Wrap(err, errors.New("readfailed"))
		}
		return (uint64(b&0x3f) << 8) | uint64(bb), false, nil

	case rdbEncVal:
		// When the first two bits are 11, the next object is encoded.
		// The next 6 bits indicate the encoding type.
		return uint64(b & 0x3f), true, nil

	default:
		switch b {
		case rdb32bitLen:
			bb, err := d.readUint32Big()
			if err != nil {
				return 0, false, err
			}
			return uint64(bb), false, nil
		case rdb64bitLen:
			bb, err := d.readUint64Big()
			if err != nil {
				return 0, false, err
			}
			return bb, false, nil
		default:
			return 0, false, errors.Errorf("Unknown length encoding %d in rdbLoadLen()", b)
		}
		// When the first two bits are 10, the next 6 bits are discarded.
		// The next 4 bytes are the length.
		// length, err := d.readUint32Big()
		// return uint64(length), false, err
	}

}

func lzfDecompress(in []byte, outlen int) []byte {
	out := make([]byte, outlen)
	for i, o := 0, 0; i < len(in); {
		ctrl := int(in[i])
		i++
		if ctrl < 32 {
			for x := 0; x <= ctrl; x++ {
				out[o] = in[i]
				i++
				o++
			}
		} else {
			length := ctrl >> 5
			if length == 7 {
				length = length + int(in[i])
				i++
			}
			ref := o - ((ctrl & 0x1f) << 8) - int(in[i]) - 1
			i++
			for x := 0; x <= length+1; x++ {
				out[o] = out[ref]
				ref++
				o++
			}
		}
	}
	return out
}

// Redis7.0使用listpack替代了ziplist
func (d *decode) readQuickList2(key []byte, expiry int64) error {
    length, _, err := d.readLength()
    if err != nil {
		return errors.Trace(err)
    }
    // https://github.com/CN-annotation-team/redis7.0-chinese-annotated/blob/7.0-cn-annotated/src/quicklist.h#L60
    // 内存占用计算，对比了github.com/HDT3213/rdb/blob/master/memprofiler/memprofiler.go
    // 节点类型为 quickListNodeContainerPlain 类的内存计算在Rpush，listpack计算在EndList
    d.info.Encoding = "quicklist2"
    d.info.Zips = length
    d.info.ListPacks = 0
    d.info.SizeOfValue = 0
    d.event.StartList(key, int64(-1), expiry, d.info)
    for length > 0 {
		length--

		containerType, _, err2 := d.readLength()
		if err2 != nil {
			return errors.Trace(err)
		}
		if int(containerType) == quickListNodeContainerPlain {
			value, err := d.readString()
		if err != nil {
			return errors.Trace(err)
		}
		d.event.Rpush(key, value, containerType)
		} else if int(containerType) == quickListNodeContainerPacked {
			listPackElements, buf := structure.ReadListPack2(d.r)
			// Quicklist是有1个或多个quickListNode组成的链表,quickListNode 自身不存数据，而是指向底层的listpack
			// listpack overhead: <total_bytes><size>...<end>，total_bytes记录的是整个 listpack 占用的总字节数，size是记录 listpack 中元素的个数
			// 因在EndList中计算listpck的内存占用，这儿进行累加
			d.info.SizeOfValue += int(buf)
			// 遍历lispck中的元素列表，对于rdr来说没用，因rdr计算的是total_bytes
			for _, value2 := range listPackElements {
				bytes := []byte(value2)
				d.event.Rpush(key, bytes, containerType)
			}

			d.info.ListPacks ++
		} else {
			log.Panicf("unknown quicklist container %d", containerType)
		}
    }
    d.event.EndList(key)
    return nil
}

// 这是 Redis7.4 最新的功能，即为 Hash 中的每个 Field 单独设置过期时间。想想确实有用，以前都是只有整个 Hash key 的过期时间
func (d *decode) readHashTtl(key []byte, expiry int64, isPre bool) error {
	rd := d.r
	var minExpire int64 = EB_EXPIRE_TIME_INVALID
	// var expireAt  int64
	//RDB_TYPE_HASH_METADATA 24 ,/* Hash with HFEs. Attach min TTL at start */
	if !isPre {
		minExpire = int64(structure.ReadUint64(rd))  // 等同于 d.ReadUint64()
		//log.Debugf("%s minExpire is %d", key, minExpire)
	}
	size := int(structure.ReadLength(rd))
	// size, _, err := d.readLength()
	// if err != nil {
	// 	return errors.Trace(err)
	// }
	d.info.Encoding = "hashtable" //临时处理
	d.event.StartHash(key, int64(size/2), expiry, d.info)
	for i := 0; i < int(size); i++ {
		// Value is absolute for 7.4RC
		expireAt := int64(structure.ReadLength(rd))
		if !isPre {
			if expireAt != 0 {
				expireAt = expireAt + minExpire - 1
			}
		}
		/*ttl, _, err := d.readLength()
		if err != nil {
			return errors.Trace(err)
		}

		if isPre {
			expireAt = int64(ttl)
		} else {
			if ttl != 0{
					expireAt = int64(ttl) + minExpire - 1
			} else {
					expireAt = 0
			}
		}*/

		fieldStr := structure.ReadString(rd)
		valueStr := structure.ReadString(rd)
		FiledBytes := []byte(fieldStr)
		ValueBytes := []byte(valueStr)
		if expireAt != 0 {
			//为 Hash 中的每个 Field 单独设置过期时间ttl
			//o.cmdC <- RedisCmd{"hpexpireat", o.key, strconv.FormatInt(expireAt, 10), "fields", "1", key}
			//因本工具暂不支持对集合类型的元素的过期时间分析，后期再处理
		}
		d.event.Hset(key, FiledBytes, ValueBytes)

	}
	d.event.EndHash(key)
	return nil
}

func (d *decode) readHashListPackTtl(key []byte, expiry int64, isPre bool) error {
	rd := d.r
	if !isPre {
		// read minExpire
		_ = int64(structure.ReadUint64(rd))
	}
	list, buf := structure.ReadListPack2(rd)
	size := len(list)
	d.info.Encoding = "listpack"
	d.info.SizeOfValue = int(buf)
	d.event.StartHash(key, int64(size/3), expiry, d.info)

	for i := 0; i < size; i += 3 {
		fieldStr := list[i]
		valueStr := list[i+1]
		FiledBytes := []byte(fieldStr)
		ValueBytes := []byte(valueStr)

		expireAt, err := strconv.ParseInt(list[i+2], 10, 64)
		if err != nil {
			log.Panicf("readHashListpackTtl parsing expireAt %s error", list[i])
		}
		if expireAt != 0 {
			//为 Hash 中的每个 Field 单独设置过期时间ttl
			//o.cmdC <- RedisCmd{"hpexpireat", o.key, strconv.FormatInt(expireAt, 10), "fields", "1", key}
			//因本工具暂时还不支持对集合的元素进行过期时间分析，后期再处理
		}
		d.event.Hset(key, FiledBytes, ValueBytes)
	}

	d.event.EndHash(key)
	return nil

}
// rdr 1.1.18 add
// readHashValkey reads Valkey 9.0's RDB_TYPE_HASH_2 format
// Format: size, then for each entry: field (string), value (string), TTL (8-byte ms timestamp)
// Valkey stores absolute expiration timestamps as int64 after each field-value pair.
// -1 (or negative) means no TTL, which is normalized to 0.
func (d *decode) readHashExValkey(key []byte, expiry int64) error {
	rd := d.r
	size := int(structure.ReadLength(rd))
	d.info.Encoding = "hashtable"
	d.event.StartHash(key, int64(size/2), expiry, d.info)

	for i := 0; i < size; i++ {
		keyStr := structure.ReadString(rd)
		valueStr := structure.ReadString(rd)
		keyBytes := []byte(keyStr)
		ValueBytes := []byte(valueStr)

		// TTL is stored as 8-byte little-endian millisecond timestamp
		expireAt := int64(structure.ReadUint64(rd))

		if expireAt < 0 {
			expireAt = 0 // valkey uses -1 to indicate no TTL
		}
		// o.cmdC <- RedisCmd{"hset", o.key, key, value}
		if expireAt != 0 {
			// o.cmdC <- RedisCmd{"hpexpireat", o.key, strconv.FormatInt(expireAt, 10), "fields", "1", key}
		}
		d.event.Hset(key, keyBytes, ValueBytes)
	}

	d.event.EndHash(key)
	return nil

}

// rdd v1.0.8 2026-01-09 add
// stream 消息队列 支持redis7增加的两个存储类型TypeStreamListPacks2=19，rdbTypeStreamListpacks3=21
// 此部分参考的 github.com/linyue515/rdr 等有精力和时间了，再对比RedisShake进行梳理改善
type StreamId struct {
	Ms       uint64 `json:"ms"`
    Sequence uint64 `json:"sequence"`
}

func (d *decode) readStreamId() (*StreamId, error) {
	ms, _, err := d.readLength()
	if err != nil {
		return nil, err
	}
	seq, _, err := d.readLength()
	if err != nil {
		return nil, err
	}
	return &StreamId{
		Ms:       ms,
		Sequence: seq,
	}, nil
}

func (d *decode) readStreamListPacks(version int, key []byte, expiry int64) error {
	/* 1. Load the number of Listpack. */
	cardinality, _, err := d.readLength()
    if err != nil {
        return errors.Trace(err)
    }
	d.info.Encoding = "stream_v2"
	d.event.StartStream(key, int64(cardinality), expiry, d.info)

	for i := uint64(0); i < cardinality; i++ {
        streamID, err := d.readString()
        if err != nil {
            return errors.Trace(err)
        }
        listpack, err := d.readString()
        if err != nil {
            return errors.Trace(err)
        }
        d.event.Xadd(key, streamID, listpack)
    }
   /* 2. Load total number of items inside the stream. stream length*/
    items, _, err := d.readLength()
    if err != nil {
        return errors.Trace(err)
    }
	/* Load the last entry ID. */
	lastID, err := d.readStreamId()
	if err != nil {
		return errors.Trace(err)
	}
    // 3. if version >=2，load Stream object metadata
    streamMeta := ""
	if version >=2 {
		firstID, err := d.readStreamId()
		if err != nil {
			return errors.Trace(err)
		}
		maxDeletedID, err := d.readStreamId()
		if err != nil {
			return errors.Trace(err)
		}
		addedCount, _, err := d.readLength()
		if err != nil {
			return errors.Trace(err)
		}
		streamMeta = fmt.Sprintf("%d-%d-%d-%d",lastID.Sequence, firstID.Sequence, maxDeletedID.Sequence, addedCount)
	}

	/* 4. Load streamgroups. */
    groups, err := d.readStreamGroups(version)
    if err != nil {
        return errors.Trace(err)
    }

	/* 5. Load IDMP (Idempotent Message Producer) configuration and entries
         for RDB_TYPE_STREAM_LISTPACKS_4 and above. */
    if version >=4 {
        // 1. 读取 IDMP duration idmpDuration
        _, _, err = d.readLength()
        if err != nil {
            return errors.Trace(err)
        }
        // 2. 读取 IDMP max entries （idmpMaxEntries）
        _, _, err = d.readLength()
        if err != nil {
            return errors.Trace(err)
        }
        // 3. 读取所有 IDMP 条目（idmpEntries）
        _, err = d.readStreamIdmpEntries()
        if err != nil {
            return errors.Trace(err)
        }

        // 4. 读取 iids_added
        _, _, err = d.readLength()
        if err != nil {
            return errors.Trace(err)
        }

        // 5. 读取 iids_duplicates
        _, _, err = d.readLength()
        if err != nil {
            return errors.Trace(err)
        }

        /* 此处可调用事件回调，将 IDMP 信息传递给上层（假设 event 接口支持） */
        // d.event.IDMPInfo(key, idmpDuration, idmpMaxEntries, idmpEntries, iidsAdded, iidsDuplicates)
	}

    d.event.EndStream(key, items, streamMeta, groups)
    return nil
}

func (d *decode) readStreamGroups(version int) (StreamGroups, error) {
    count, _, err := d.readLength()
    if err != nil {
        return nil, err
    }
    /* Consumer groups loading */
    groups := make(StreamGroups, 0, count)
    for i := 0; i < int(count); i++ {
        group := &StreamGroup{
            Pending:   make([]*StreamPendingEntry, 0),
            Consumers: make([]*StreamConsumerData, 0),
        }

        // 读取组名
        group.Name, err = d.readString()
        if err != nil {
            return nil, err
        }

        // 读取最后ID
        gIDms, _, err := d.readLength()
        if err != nil {
            return nil, err
        }
        gIDseq, _, err := d.readLength()
        if err != nil {
            return nil, err
        }
        /* Load group offset. */
        entriesRead, _, err := d.readLength()
        if err != nil {
            return nil, err
        }
        group.LastEntryId = fmt.Sprintf("%d-%d-%d", gIDms, gIDseq,entriesRead)
        // 读取PEL
        pelSize, _, err := d.readLength()
        if err != nil {
            return nil, err
        }
        for j := 0; j < int(pelSize); j++ {
            ms,err:=d.readUint64()
            if err !=nil {
                return nil, err
            }
            seq,err := d.readUint64()
            if err !=nil {
                return nil, err
            }
            streamID :=&StreamId{
                Ms: ms,
                Sequence: seq,
            }
            deliveryTime, err := d.readUint64()
            if err != nil {
                return nil, err
            }
            deliveryCount, _, err := d.readLength()
            if err != nil {
                return nil, err
            }
            group.Pending = append(group.Pending, &StreamPendingEntry{
                ID:            streamID,
                DeliveryTime:  deliveryTime,
                DeliveryCount: deliveryCount,
                IsNack:        false, // 默认非NACK，后续根据版本更新
            })
        }

        // 读取消费者
        consumersNum, _, err := d.readLength()
        if err != nil {
            return nil, err
        }
        // 构建消费者ID集合，用于后续NACK验证
        consumerIDs := make(map[[2]uint64]bool) // key: [ms, seq]
        for j := 0; j < int(consumersNum); j++ {
            consumer := &StreamConsumerData{
                Pending: make([]*StreamConsumerPendingEntry, 0),
            }
            consumer.Name, err = d.readString()
            if err != nil {
                return nil, err
            }
            consumer.SeenTime, err = d.readUint64()
            if err != nil {
                return nil, err
            }
            // consumer->active_time = rdbLoadMillisecondTime(rdb,RDB_VERSION);
            if version >=3 {
                consumer.ActiveTime,err = d.readUint64()
                if err != nil {
                    return nil, err
                }
            }
            /* Consumer PEL */
            pelSize, _, err := d.readLength()
            if err != nil {
                return nil, err
            }
            for k := 0; k < int(pelSize); k++ {
                id := make([]byte, 16)
                if _, err := io.ReadFull(d.r, id); err != nil {
                    return nil, err
                }
                consumer.Pending = append(consumer.Pending, &StreamConsumerPendingEntry{
                    ID: id,
                })
                // 将ID解析为ms和seq，加入消费者ID集合（用于NACK验证）
                ms := binary.LittleEndian.Uint64(id[0:8])
                seq := binary.LittleEndian.Uint64(id[8:16])
                consumerIDs[[2]uint64{ms, seq}] = true
            }

            group.Consumers = append(group.Consumers, consumer)
        }

        /* For RDB_TYPE_STREAM_LISTPACKS_5 and above, load the NACK */
        // ----- version >= 5：处理 NACK zone -----
        if version >= 5 {
            nackedCount, _, err := d.readLength()
            if err != nil {
                return nil, err
            }
            for n := uint64(0); n < nackedCount; n++ {
                rawID := make([]byte, 16)
                if _, err := io.ReadFull(d.r, rawID); err != nil {
                    return nil, err
                }
                ms := binary.LittleEndian.Uint64(rawID[0:8])
                seq := binary.LittleEndian.Uint64(rawID[8:16])

                // 在组 PEL 中查找匹配条目
                var foundEntry *StreamPendingEntry
                for _, entry := range group.Pending {
                    if entry.ID.Ms == ms && entry.ID.Sequence == seq {
                        foundEntry = entry
                        break
                    }
                }
                if foundEntry == nil {
                    return nil, fmt.Errorf("NACK entry ID %d-%d not found in group PEL", ms, seq)
                }
                // 检查该条目是否已被某个消费者拥有（不应该出现在消费者PEL中）
                if _, ok := consumerIDs[[2]uint64{ms, seq}]; ok {
                    return nil, fmt.Errorf("NACK entry ID %d-%d has consumer assigned, corrupt RDB", ms, seq)
                }
                // 标记为 NACK
                foundEntry.IsNack = true
            }
            // 可选：deep_integrity_validation 可在此处遍历所有 PEL 条目，
            // 确保非 NACK 的条目都有消费者，但本解析器可省略。
        }

        groups = append(groups, group)
    }
    return groups, nil
}

// IDMP 条目结构
type IDMPEntry struct {
    IID      string   // 幂等 ID（key）
    StreamID *StreamId // 对应的消息 ID
}

// 读取 IDMP 条目列表
func (d *decode) readStreamIdmpEntries() ([]*IDMPEntry, error) {
    // 读取条目数量
    count, _, err := d.readLength()
    if err != nil {
        return nil, err
    }
    entries := make([]*IDMPEntry, 0, count)
    for i := uint64(0); i < count; i++ {
        // 读取 IID（字符串）
        iidBytes, err := d.readString()
        if err != nil {
            return nil, err
        }
        // 读取对应的 Stream ID（ms 和 sequence）
        ms, err := d.readUint64()
        if err != nil {
            return nil, err
        }
        seq, err := d.readUint64()
        if err != nil {
            return nil, err
        }
        entries = append(entries, &IDMPEntry{
            IID:      string(iidBytes),
            StreamID: &StreamId{Ms: ms, Sequence: seq},
        })
    }
    return entries, nil
}

// skipKeyMeta reads and discards key metadata classes (RDB 13).
// Format: [numClasses] then for each class: [4-byte classSpec] [module-value]
func (d *decode) skipKeyMeta()  error {
	numClasses, _, err := d.readLength()
	if err != nil {
		return fmt.Errorf("read key meta numClasses failed: %v", err)
	}
	for i := uint64(0); i < numClasses; i++ {
		// read 4-byte classSpec（模块类型 ID）
		_, err := io.ReadFull(d.r, d.intBuf[:4])
		if err != nil {
			return fmt.Errorf("read key meta classSpec failed: %v", err)
		}

		// skip the module-encoded value
        if err := structure.SkipModuleAuxData(d.r); err != nil {
            return fmt.Errorf("skip key meta value failed: %v", err)
        }

	}
	return nil
}
