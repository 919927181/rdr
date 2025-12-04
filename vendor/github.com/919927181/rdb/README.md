# rdb [![Build Status](https://travis-ci.org/cupcake/rdb.png?branch=master)](https://travis-ci.org/cupcake/rdb)

rdb is a Go package that implements parsing and encoding of the
[Redis](http://redis.io) [RDB file
format](https://github.com/sripathikrishnan/redis-rdb-tools/blob/master/docs/RDB_File_Format.textile).

This package was heavily inspired by
[redis-rdb-tools](https://github.com/sripathikrishnan/redis-rdb-tools) by
[Sripathi Krishnan](https://github.com/sripathikrishnan).

[**Documentation**](http://godoc.org/github.com/cupcake/rdb)

## Installation

```
go get github.com/cupcake/rdb
```

###  Fok Change

```

为了更改rdb源码，改成我的github仓库下，以便更快的解决redis7支持的问题。

参考阿里云 RedisShake redis数据迁移工具，支持redis7的数据类型

针对redis7、8版本，rdb文件解析主要是解决listpack数据类型问题。鉴于 redis stream 用于消息队列，我们通常不用redis作为mq，因此stream增加的类型未处理。  
```