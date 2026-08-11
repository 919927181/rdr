![license](https://img.shields.io/github/license/919927181/rdr)
![download](https://img.shields.io/github/downloads/919927181/rdr/total)
[![Build Status](https://github.com/919927181/rdr/actions/workflows/go.yml/badge.svg)](https://github.com/919927181/rdr/actions?query=branch%3Amaster)
[![Go Report Card](https://goreportcard.com/badge/github.com/919927181/rdr)](https://goreportcard.com/report/github.com/919927181/rdr)


## About（介绍）

RDR (redis data Reveal) is a tool for offline analysis of redis rdb files. Through it, you can quickly discover bigkeys, help you grasp the occupation and distribution of keys in memory, learn which keys are growing infinitely (through key expiration time or quantity). It provides data support for your optimization operations and helps you avoid problems such as insufficient memory and performance degradation caused by key skew.

RDR(redis data Reveal)是一个用于离线分析 redis rdb 文件的工具。从类型、元素数量、key过期时间和数据大小，这4个角度分析，帮助您掌握redis存了哪些 key，是什么类型，Key在内存中的占用和分布情况（哪些 key 内存占用最多、元素个数多）以及key的过期状况等。有助于您定位遇到的redis使用问题，能为您的优化操作提供数据支持，帮助您避免因Key倾斜（导致集群内存分布不均）引发的内存不足、性能下降等问题发生。

常见用途，如找出 bigkey，哪些key无限制的增长，用于排查redis内存占用高或CPU负载高，甚至系统崩溃问题原因。


 功能：
  - 统计信息展示（command:show）：以网页形式展示RDB文件的统计报告（例如 Top 300 大Key列表、key名前缀统计分析等）。
  - 统计信息保存(command:dump2file)：除了在线网页展示外，还可以将统计信息保存到文件。
  - 获取所有key（command:keys）：从RDB文件中解析出全部键名以及属性信息（数据类型、内存大小、元素数量、过期时间、所属db等），保存到文件，以便自行分析。
 
 备注：如果想知道集合类中最大元素是谁，则保存到文件，LargestKeys>FieldOfLargestElem字段就是。
 
 特点：
  - 安全无扰：分析过程完全在 RDB 备份文件上进行，对线上Redis实例零影响。
  - 使用方便：提供了linux和windows可执行文件，不需要安装；一键生成内存健康报告，在线图形化展示更直观。
  - 高效解析：RDR由golang实现，解析速度非常快，一个10G的rdb文件，v1.1.4版本起，在有前缀数量限制下，用时30秒。
  - 结果精准：结果反映的是RDB快照生成时刻的精确状态。
  - 庖丁解牛：深入RDB文件结构与LRU元数据原理，犹如为缓存做了一次精准的“核磁共振”检查。

注意：生产环境下慎行，以免对线上实例产生性能影响，我们可以在从节点或拷贝rdb文件到测试机上去分析。

## Fork（基于）

- 本项目基于 xueqiu/rdr 开源项目开发，实现了对redis7+的支持。
- 核心依赖包 919927181/rdb（源自dongmx/rdb）解析redis rdb 文件。

注：xueqiu/rdr是雪球公司基于redis-rdb-tool开源项目开发的，更新维护停止在 2019 年 10 月 9 日。

在此，对原开源项目作者，以及提供issues、pr的朋友们，表示感谢。


## Support For Redis（redis 版本支持）

  - RDR V1.0.3  支持 Redis6
  - RDR V1.0.5  支持 Redis7.0+, Valkey 8.x
  - RDR V1.1.18 支持 Redis8.8+, Valkey(9.0+)
  
``` 
redis rdb版本：
# Redis-8.10.0 :    `dump_rdb_version=15`
# Redis-8.8.1 :     `dump_rdb_version=15`
# Redis-8.6.5  :    `dump_rdb_version=13`
# Redis-8.4 :       `dump_rdb_version=12`
# Redis-7.4.x :     `dump_rdb_version=12`
# Redis-7.2.x :     `dump_rdb_version=11`
# Redis-7.0.x :     `dump_rdb_version=10`
# Redis-5.x ~ 6.x : `dump_rdb_version=9`
# Redis-4.0.x :     `dump_rdb_version=8`
# Redis-3.2 :       `dump_rdb_version=7`
# Redis-2.x ~ 3.0 : `dump_rdb_version=6`
``` 
    
 备注：
  - RDR的核心依赖是 rdb 文件解析，不同版本redis，其 rdb 文件存在差异，也会增加新的数据类型，存在数据兼容性问题。
  - 如果解析高版本时出现错误，可以在测试实例上删除不支持的类型key，或使用 RedisShake 数据迁移工具，迁到低版本实例后，再用rdr进行分析。

## Change（重点版本）
- caiqing0204：增加了key所属DB，这样可以更直观的查看key元信息。
- 泰山李工（我）：
   - v1.0.2
     - 将依赖 github.com/dongmx/rdb 中的rdbVersion 由9改成20【2025-11-08】
     - 修改html布局、将标题英文改为中文 【2025-11-08】
	 
   - v1.0.3 
     - 升级chartjs版本，实现图表tip时，显示更人性化的数字【2025-11-13】
     - 将2021年3月 至 2023年7月，原项目 [xueqiu/rdr](github.com/xueqiu/rdr/pulls) 的pulls，同步过来、并解决完毕。
	 
   - v1.0.5 
     - 完成redis7+支持，主要解决了redis7.x底层存储类型使用listpack替代ziplist的解析问题。
    
   - v1.1.4
     - 优化key前缀统计的处理逻辑，增加动态缩容机制，防止创建海量的前缀对象因内存不足，导致分析不完【Heachy PR, 2026-03-17】
	 - 优化后：在前缀数量限制下，一个10G的rdb文件，用时30秒。以前没限制时，需要4分钟左右。 
	 
   - v1.1.5
     - 在线分析报告（command:show），增加Key过期剩余时间分析、key数据大小区间分析
	 - 修复已知问题，如按key元素个数据分析的前端赋值重复问题，通过带有函数的json对象深拷贝解决


## Usage（使用）

```
USAGE:
   rdr [global options] command [command options] [arguments...]

VERSION:
   vx.x.x

COMMANDS:
     dump2file dump statistical report of rdbfile to file(./rdb-report-xxx.json).
     show      show statistical report of rdbfile by webpage
     keys      write all keys of rdbfile to file（./rdb-all-keys-xxx.txt）.
     help, h   show a list of commands or help for one command

GLOBAL OPTIONS:
   --help, -h     show help
   --version, -v  print the version
```

```
NAME:
   rdr show - show statistical information of rdbfile by webpage

USAGE:
   rdr show [command options] FILE1 [FILE2] [FILE3]...

OPTIONS:
   --port value, -p value  Port for rdr to listen (default: 8080)
```


### linux下使用说明

```
1.从releases中，下载 linux 下的可执行文件

2.创建目录
# mkdir -p /tmp/rdb/
# cd /tmp/rdb/

3.使用redis的bgsave命令导出全量文件(rdb文件)

4.然后把rdr工具、redis的数据库文件.rdb上传到该目录下
给工具赋予执行权限
# chmod a+x ./rdr*

5.进行分析
# GOGC=200 ./rdr-linux show -p 8099 *.rdb
注意,如果你的rdb文件比较大（1G+）,建议一次只分析一个rdb文件
    如果rdb文件大，那么cpu使用率就会过高，此时我们调整GOGC，默认100，提高值(200-400)可降低GC频率，减少CPU占用但会增加峰值内存使用

6.防火墙端口放行
     For Ubuntu\Debian：sudo ufw allow 8099/tcp  &&  sudo ufw reload
     For Redhat\Centos：
          sudo firewall-cmd --zone=public --add-port=8099/tcp --permanent
          sudo firewall-cmd --reload
		   
7.查看分析报告，浏览器访问 http://your-host:8099/
```

### windows下使用说明
```
 1. 从releases中，下载 windows 下的可执行文件

 2. 将rdr工具、redis的数据库文件.rdb拷贝到某个文件夹下

 3. 在该文件夹下空白的地方，ctrl+鼠标右击，打开 cmd 执行：
    > .\rdr-win64.exe show -p 8099 dump.rdb

 4. 查看分析报告，打开浏览器访问 http://localhost:8099 
```

## Exapmle 使用示例
```
1.通过网页显示rdb file的统计报告
$ GOGC=200 ./rdr-linux show -p 8099 dump.rdb
# 从v1.1.3版本起：
  - 支持指定获取最大的 N 个键，最大500; 
  - 支持指定内存大小过滤参数-s，用于导出大于阈值的大 key，小于阈值的key被过滤掉，单位为B/KB/MB/GB
$ GOGC=200 ./rdr-linux show -p 8099 -n 100 -s 10kb dump.rdb

# 从v1.1.4版本起：
  - 支持使用 -sep 参数传入前缀分割符，默认为':;,_- '。
  - 前缀分析时，默认会进行动态收缩，防止前缀对象创建越多（xxx业务名:uuid），内存使用就越多，内存不足就会导致执行不完。
$ GOGC=200 ./rdr-linux show -p 8099 -pn 500 -psn 5000 -pmn 50000 dump.rdb
参数说明：
  - spa true 前缀分析时，前缀对象数量不进行动态缩容；默认fasle
  - pn  前缀Top数量，默认值500
  - pmn 前缀容器的容量上限，可以理解为最大水位线，超标时就要泄洪到正常水位线，默认值50000，若你主机内存小，则设小点
  - psn 前缀容器的预缩容数量，可以理解为正常水位线，默认值5000，建议设为容量上限的 10%\30%\50%，避免频繁触发
  
```
Note that the memory usage is approximate.
<img width="1080" height="793" alt="image" src="https://github.com/user-attachments/assets/a3de1f50-5649-4a6c-838f-e105c9994661" />

```
2.将统计报告写到文件（当前目录/rdb-report-xxx.json）
# 如果你想知道集合类中的哪个元素最大，见报告中的LargestKeys>FieldOfLargestElem字段。
$ GOGC=200 ./rdr-linux  dump2file  dump.rdb
# 从v1.1.3版本起：
  - 支持指定获取最大的 N 个键，最大500; 
  - 支持指定内存大小过滤参数-s，用于导出大于阈值的大 key，小于阈值的key被过滤掉，单位为B/KB/MB/GB
$ GOGC=200 ./rdr-linux  dump2file -n 100 -s 10kb  dump.rdb

# 从v1.1.4版本起：
  - 支持使用 -sep 参数传入前缀分割符，默认为':;,_- '。
  - 前缀分析时，默认会进行动态收缩，防止前缀对象创建越多，内存使用就越多，内存不足就会导致执行不完。
$ GOGC=200 ./rdr-linux dump2file -pn 500 -psn 5000 -pmn 50000 dump.rdb
参数说明：
  - spa true 前缀分析时，前缀对象数量不进行动态缩容；默认fasle
  - pn  前缀Top数量，默认值500
  - pmn 前缀容器的容量上限，可以理解为最大水位线，超标时就要泄洪到正常水位线，默认值50000，若你主机内存小，则设小点
  - psn 前缀容器的预缩容数量，可以理解为正常水位线，默认值5000，建议设为容量上限的 10%\30%\50%，避免频繁触发
  
```

```
3.解析出所有key元信息，输出到文件（当前目录/rdb-all-keys-xxx.txt），以便自行分析。
$ GOGC=200 ./rdr-linux keys dump.rdb
key,type,encoding,size,humanizeSize,numOfElem,expiration,expire_seconds,lruIdle,lfuFreq,db
student:1:name, string, string, 100, 100 B, 8, , -1, 0, 0
colors, set, listpack, 94, 94 B, 2, , -1, 0, 0

附-mysql建表语句：
CREATE TABLE rdb_keys_infor (
   `id`  int  PRIMARY KEY AUTO_INCREMENT,
    `key` varchar(300)  NOT NULL COMMENT 'key名',
    `type` varchar(30)  NOT NULL COMMENT '类型',
    `encoding` varchar(30)  NOT NULL COMMENT '底层类型',
    `size` int NOT NULL COMMENT '内存使用',
    `humanizeSize` varchar(30) NOT NULL COMMENT '内存使用',
    `numOfElem` int NOT NULL COMMENT '元素数量',
    `expiration`  varchar(30)   COMMENT '过期时间',
    `expire_seconds` int NOT NULL COMMENT '过期剩余时间，单位秒。-1表示永不过期，0表示已经过期',
    `lruIdle`  varchar(30)   COMMENT '最后一次方式时间',
    `lfuFreq` int NOT NULL COMMENT '访问频率',
    `db` int NOT NULL COMMENT 'db'
) ENGINE = InnoDB COMMENT = 'redis key信息表';

备注：maxmemory-policy配置的淘汰策略：
  - 如果策略是volatile-lru或allkeys-lru，记录的是该键距离上次访问的空闲时间（lruIdle），存储了一个24比特位的“LRU时钟”值，单位是秒。
    - 我们可以根据rdb的创建时间，来推算出键的最后一次访问时间 ≈ RDB文件的创建时间（ctime） - idle秒数 
    - 不要依赖此公式进行精确的、长周期的时间推算，因为24位计数器周期是194天。
  - 如果策略包含LFU（如volatile-lfu），记录的是访问频率（lfuFreq）。

```

## 常见问题

```
Q:rdr 一直执行不完，free -m 显示内存不足（看available列：系统真正可用的内存），怎么办？
A:使用符合自身实情的分隔符或将前缀分析的动态收缩机制的参数调小，防止因创建的前缀对象太多，导致内存不足。
  建议使用8C\16GB的机器。如果不能解决，则使用 pprof 分析内存占用或联系我们。

Q：为什么rdr算的内存大小和使用 memory usage 命令查的结果不一致？
A：不一致是预期内的正常现象，rdr是基于RDB文件进行的离线估算，其实命令查的结果也是非精准值，在容量治理和故障排查的“战场”上，大可放心使用。

Q：如何处理报错decode rdbfile error: rdb: unknown object type 116 for key？
A：该报错表示实例中存在非标准或新版本增加的数据结构，暂不支持分析，你可以还原到测试实例删除后再进行分析。

Q：为什么Redis缓存分析中String类型Key的元素数量和元素长度是一样的？
A：在Redis缓存分析中，针对String类型的Key，其元素数量就是其元素长度。集合类。

Q：Redis缓存分析的前缀分隔符是什么？
A：目前Redis缓存分析的前缀分隔符，默认为":;,_-@#. "，V1.1.4起支持传递参数。

Q：通常redis集群只会用到db0,单例中可能会用多个槽。那么当不同db里有相同前缀的key时，前缀分析列表该如何显示所属db？
A: 从v1.0.9版本起，将所有所属的db都进行了显示，多个以逗号隔开。

Q：想看每个db或某个db的占用内存大小，怎么看？
A: 导出所有key信息后自行分析。

Q：rdr是如何计算的Key过期时间？
A: key的过期时间是时间戳，不是ttl。rdr 根据与 rdb 创建时间（AUX元属性之ctime）的差值，进行的分布分析。

Q：什么是大 key?
A：没有硬性规定的，通常字符串类型超过10KB，集合类的元素个数超过5000个，认为是Big key。有的公司约定单 key 体积不超1MB，集合类型二级的条目个数少于1千个性能最佳，最好不要超过1万个。

Q：过期时间设置多少比较好？
A: 缓存时间设置1小时内最好，最好不要超过24小时

```


## RDR 开发

1. 文件目录结构

```
\decoder\
   |-- decoder.go       # 对github.com/919927181/rdb/decoder.go中的interface方法实现，rdb解析各类型数据时，会调用相应的方法，进行计数。
   |-- memprofiler.go  # 内存分析器
\dump\
   |-- dump.go          # 将rdb文件统计信息输出到STDOUT或文件
   |-- keys_export.go  # 将获取所有key及属性信息输出到File，以便自行分析之需要
\static  # 以html方式展示分析报告的静态资源文件
\views  # html 前端页面代码

```

2. 如果你想修改redis rdb 文件解析插件源码，可以pr到github.com/919927181/rdb

3. 如果你需要修改html（rdr报告的网页），你需要安装go-bindata 或 go-bindata-assetfs

    安装手册可参考 https://blog.csdn.net/qq_67017602/article/details/130742316
	
	安装 go-bindata 
      go get -u github.com/go-bindata/go-bindata/...
      当go版本大于1.17的时候，使用这个命令：
      go install -a -v github.com/go-bindata/go-bindata/...@latest
	  
	或安装 go-bindata-assetfs
      go install -a -v github.com/elazarl/go-bindata-assetfs/...@latest

4. 打包
   
```
 1.如果改动了静态资源（css\js\html），需要使用go-bindata将静态资源文件嵌入到go文件里
    go-bindata -prefix "static/" -o=static/static.go -pkg=static -ignore static.go static/... 
    go-bindata -prefix "views/" -o=views/views.go -pkg=views -ignore views.go views/...
 
 2. 在编译前自动化生成某类代码，它常用于自动生成代码，我一般是直接执行打包命令
    go generate
	
 3. 在windows下打包，编译出 linux 下的可执行文件，在项目根目录下，打开cmd，执行以下命令
    set CGO_ENABLED=0
    set GOOS=linux
    set GOARCH=amd64
    go build -o rdr-linux  main.go
	
	编译出Windows下的exe文件
	set CGO_ENABLED=0
	set GOOS=windows
	set GOARCH=amd64
	go build -o rdr-win64.exe  main.go

```


## RDB 开发

 rdr工具的核心部分就是rdb文件解析，作为开发者，我们可以通过以下几个途径来掌握相关知识：

1. 大部分 rdb 文件的解析都是按照 [redis-rdb-tools](https://github.com/sripathikrishnan/redis-rdb-tools/wiki/Redis-RDB-Dump-File-Format)  和 [cupcake/rdb](https://github.com/cupcake/rdb) 来的，
   RDB 文件格式说明：https://www.cnblogs.com/Finley/p/16251360.html ，RDB解析器源码可以看 [919927181/rdb](https://github.com/919927181/rdb) 、 [HDT3213/rdb](https://github.com/HDT3213/rdb) 等

2. Redis迁移工具RedisShake（语言golang），由阿里云 [Tair 团队](https://github.com/tair-opensource)长期维护， 从 RDB 文件中读取数据写入目标端，我们可以参考它这块的代码。
   项目地址：https://github.com/tair-opensource/RedisShake
   rdb.go源码地址：https://github.com/tair-opensource/RedisShake/tree/v4/internal/rdb

3. 对照 redis 源码

```
  rdb.c 文件：https://github.com/redis/redis/blob/7.0-rc3/src/rdb.c     // RDB 文件读写，行1736：robj *rdbLoadObject
  rdb.h 文件：https://github.com/redis/redis/blob/unstable/src/rdb.h   // RDB version、RDB object types 等定义

```


## 贡献

欢迎来自开源社区的贡献。对于重大变更，请先开一个 issue 来讨论你想要改变的内容，可以加我微信（Sd-LiYanJing）入群讨论。

特别感兴趣的是：

 1. 随着redis版本变化，增加新类型的解析支持
 2. 优化、改善代码，提升性能
 

## 交流群

添加微信（Sd-LiYanJing），备注rdr，即可进群

## 最后

对本项目参考的开源项目和资料，再次表示感谢，感谢大家对开源社区的贡献！ 

感谢诸位兄弟/姐妹对此项目的支持。因个人维护精力有限，欢迎Pr或加入项目组，让该工具得以持续！


## License

This project is under Apache v2 License. See the [LICENSE](LICENSE) file for the full license text.