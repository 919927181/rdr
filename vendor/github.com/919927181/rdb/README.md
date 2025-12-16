# rdb [![Build Status](https://travis-ci.org/cupcake/rdb.png?branch=master)](https://travis-ci.org/cupcake/rdb)

rdb is a Go package that implements parsing and encoding of the
[Redis](http://redis.io) [RDB file
format](https://github.com/sripathikrishnan/redis-rdb-tools/blob/master/docs/RDB_File_Format.textile).

This package was heavily inspired by
[redis-rdb-tools](https://github.com/sripathikrishnan/redis-rdb-tools) by
[Sripathi Krishnan](https://github.com/sripathikrishnan).

[**Documentation**](http://godoc.org/github.com/cupcake/rdb)

rdb是一个Go包，实现了对redis rdb文件的解析和编码。

 - 基于github.com/dongmx/rdb进行的二次开发，实现了redis7+支持。
 - 本包支持redis rdb 版本为 1 <= version <= 12
 
 注：dongmx/rdb 源自github.com/cupcake/rdb，仅支持redis6，2019年10月停止维护。

## Installation

```
go get github.com/cupcake/rdb
```

## 开发

rdb 文件解析，请阅读 [Redis-RDB-Dump-File-Format](https://github.com/sripathikrishnan/redis-rdb-tools/wiki/Redis-RDB-Dump-File-Format) 和 [Documentation For cupcake/rdb](http://godoc.org/github.com/cupcake/rdb)

- redis7+支持，\core\下的源码参考了 RedisShake ，它是阿里云 [Tair 团队](https://github.com/tair-opensource) 积极维护的一个用于处理和迁移 Redis 数据的工具。在此，表示感谢！

- redis7+，rdb文件解析主要是解决listpack数据类型问题。鉴于 redis stream 用于消息队列，我们通常不用redis作为mq，因此stream增加的类型未处理。 


```
//Decode parses a RDB file from r and calls the decode hooks on d.
//Decode 从 r 解析 RDB 文件并调用 d 上的解码挂钩.
func Decode(r io.Reader, d Decoder) error

//Decode a byte slice from the Redis DUMP command. The dump does not contain the database, key or expiry, so they must be included in the function call (but can be zero values).
//从 Redis DUMP 命令中解码字节片。转储不包含数据库、密钥或过期时间，因此它们必须包含在函数调用中.
func DecodeDump(dump []byte, db int, key []byte, expiry int64, d Decoder) error

//A Decoder must be implemented to parse a RDB file. 必须实现解码器来解析 RDB 文件。
type Decoder interface {
	// StartRDB is called when parsing of a valid RDB file starts.
	StartRDB()
	...
}

```


## 常见问题

Q：为什么使用命令（memory usage ）获取的和rdr算的总是不一致
A：Key和value所对应的struct和指针大小。在jemalloc分配后，字节对齐部分所占用的大小也会计算在used_memory中
   无论是用命令还是rdr都计算了这两块，为什么不一致？可读下 https://blog.csdn.net/f80407515/article/details/122387859

Q：如何处理报错decode rdbfile error: rdb: unknown object type 116 for key？
A：该报错表示实例中存在非标准或新版本增加的数据结构，暂不支持分析，你可以在还原到测试实例删除后再进行分析。

Q：为什么Redis缓存分析中String类型Key的元素数量和元素长度是一样的？
A：在Redis缓存分析中，针对String类型的Key，其元素数量就是其元素长度。

Q：Redis缓存分析的前缀分隔符是什么？
A：目前Redis缓存分析的前缀分隔符是按照固定的前缀:;,_-+@=|# 区分的字符串。

Q：各key的内存占用为什么比[HDT3213/rdb](https://github.com/HDT3213/rdb)算的大28？
A：HDT3213/rdb V.1.3.0没有计算lru_bits占用，lru_bits默认占用24比特位，而本工具将起计算在内了，请看源码d.m.TopLevelObjOverhead。


### 贡献

欢迎社区的贡献。对于重大变更，请先开一个 issue 来讨论你想要改变的内容。如果想共同维护此项目，可以加我微信（Sd-LiYanJing）。

特别感兴趣的是：

 1. 随着redis版本变化，增加新类型的读解析支持
 2. 优化、改善代码，提升性能



