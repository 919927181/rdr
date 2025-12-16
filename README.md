RDR: redis data reveal
=================================================

## About（介绍）

RDR (redis data Reveal) is a tool for offline analysis of redis rdb files. Through it, you can quickly discover bigkeys, help you grasp the occupation and distribution of keys in memory, learn which keys are growing infinitely (through key expiration time or quantity). It provides data support for your optimization operations and helps you avoid problems such as insufficient memory and performance degradation caused by key skew.

- RDR(redis data Reveal)是一个用于离线分析 redis rdb 文件的工具，通过它，可以快速发现实例中的bigkey，帮助您掌握Key在内存中的占用和分布、得知哪些key在无限增长（通过Key过期时间或数量）等信息，为您的优化操作提供数据支持，帮助您避免因Key倾斜引发的内存不足、性能下降等问题。
- RDR由golang实现的，速度上比较快。


## Fork（基于）

This repository is fork  from github.com/xueqiu/rdr.  The requrie rdb file parse is github.com/dongmx/rdb ，in this project has been replaced  github.com/919927181/rdb

- 本项目基于 xueqiu/rdr 开源项目开发，而xueqiu/rdr是雪球公司基于redis-rdb-tool开源项目开发的，更新维护停止在 2019 年 10 月 9 日。
- 核心依赖包 919927181/rdb（基于dongmx/rdb）解析redis rdb 文件。

在此，对原开源作者，以及提供灵感、pr的朋友们表示感谢。


## Redis Version Support（redis 版本支持）

支持redis rdb 文件版本为 1 <= version <= 12

  - RDR V1.x 支持 Redis6（redis 5.x ~ 6.x ，rdb 的版本是 9 ) 
  - RDR V2.x 支持 Redis7.0+（rdb 文件版本 10~12，mysql8.0的rdb版本是12）
  
 备注：
  - 针对redis7+版本，rdb文件解析主要是解决listpack数据类型问题。鉴于 redis stream 用于消息队列，我们通常不用redis作为mq，因此stream增加的类型未处理。  
  - RDR的核心依赖是 rdb 文件解析，不同版本的 redis，其 rdb 文件存在差异，也会增加新的数据类型，存在数据兼容性问题。
    - 如果解析高版本的redis时出现错误，可以尝试通过 RedisShake 数据迁移工具，将redis7 RDB数据迁移到redis6下，然后再用rdr\进行分析。


## Change（变更）
- caiqing0204：增加了key所属DB，这样可以更直观的查看key元信息。
- 泰山李工（Me）：
   - v1.0.2
     - 将依赖 github.com/dongmx/rdb 中的rdbVersion 由9改成20【2025-11-08】
     - 修改html布局、将标题英文改为中文 【2025-11-08】
	 
   - v1.0.3 
     - 升级chartjs版本，实现图表tip时，显示更人性化的数字【2025-11-13】
     - 将2021年3月 至 2023年7月，在原作者 github.com/xueqiu/rdr/pulls，除过滤小key外的其他pulls，均同步过来、并解决完毕。
	 - 遗留问题： redis3.2+新引入的encoding为quicklist作为list的基础类型，list的元素个数是个超大数字，在分析时可能溢出
	 
   - v2.x 
     - 支持redis7,主要解决了redis7.x底层存储类型使用listpack替代ziplist的解析问题。


## Usage（使用）

```
NAME:
   rdr - a tool to parse redis rdbfile

USAGE:
   rdr [global options] command [command options] [arguments...]

VERSION:
   v0.0.1

COMMANDS:
     show     show statistical information of rdbfile by webpage
     keys     get all keys from rdbfile
     help, h  Shows a list of commands or help for one command

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

```
NAME:
   rdr keys - get all keys from rdbfile

USAGE:
   rdr keys FILE1 [FILE2] [FILE3]...
```

```
1）在windows 下打包，编译出 linux 下的可执行文件，在项目根目录下，打开cmd，执行以下命令
    set GOOS=linux
    set GOARCH=amd64
    go build -o rdr-linux  main.go

2）创建目录
# mkdir -p /tmp/redisdb/
# cd /tmp/redisdb/

3）然后把rdr工具、redis的数据库文件.rdb上传到该目录下

给工具赋予执行权限
# chmod a+x ./rdr*

4）运行
# ./rdr-linux show -p 8099 *.rdb

5）防火墙端口放行
     For Ubuntu\Debian：sudo ufw allow 8099/tcp  &&  sudo ufw reload
     For Redhat\Centos：
          sudo firewall-cmd --zone=public --add-port=8099/tcp --permanent
           sudo firewall-cmd --reload
		   
6）查看分析结果，浏览器访问 http://yourip：8099/
```

## Exapmle
```
$ ./rdr show -p 8080 *.rdb
```
Note that the memory usage is approximate.
<img width="1155" height="612" alt="image" src="https://github.com/user-attachments/assets/a8b16a78-b232-4282-b2ff-781f0cc87504" />

```
$ ./rdr keys example.rdb
portfolio:stock_follower_count:ZH314136
portfolio:stock_follower_count:ZH654106
portfolio:stock_follower:ZH617824
portfolio:stock_follower_count:ZH001019
portfolio:stock_follower_count:ZH346349
portfolio:stock_follower_count:ZH951803
portfolio:stock_follower:ZH924804
portfolio:stock_follower_count:INS104806
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


## RDR 开发

1. 文件目录结构

```
\decoder\
   |-- decoder.go       # 调用直接依赖github.com/919927181/rdb的方法，读取rdb数据
   |-- memprofiler.go  # 内存分析器
\dump\
   |-- dump.go  # 转储rdb文件统计信息
\static  # 以html展示结果时，需要的静态资源文件
\views  # html 前端页面代码

```


2. 如果你想修改redis rdb 文件解析插件的源码，可以pr到github.com/919927181/rdb

   你可以直接修改下载到本地的依赖 \vendor\github.com\919927181\rdb，调试成功后，再进行pr 或创建\并引用自己的rdb依赖


3. 如果你需要修改html

    你需要安装go-bindata，安装手册可参考 https://blog.csdn.net/qq_67017602/article/details/130742316

4. 打包
   
```
 1.如果改动了静态资源（css\js\html），需要使用go-bindata将静态资源文件嵌入到go文件里

 //go-bindata -prefix "static/" -o=static/static.go -pkg=static -ignore static.go static/...
 
 //go-bindata -prefix "views/" -o=views/views.go -pkg=views -ignore views.go views/...
 
 2. 作用是在编译前自动化生成某类代码;它常用于自动生成代码
 
 go generate
 ```


5. 构建，输出linu下的可执行文件

见上面的Usage（使用）


## RDB 开发

 rdr工具的核心部分就是rdb文件解析，作为开发者，我们可以通过以下几个途径来掌握相关知识：

1）大部分 rdb 文件的解析都是按照 https://github.com/sripathikrishnan/redis-rdb-tools/wiki/Redis-RDB-Dump-File-Format  和 github.com/cupcake/rdb 来的，
   RDB 文件格式说明：https://www.cnblogs.com/Finley/p/16251360.html   ，完整解析器源码在 github.com/HDT3213/rdb

2）Redis迁移工具RedisShake（语言golang），功能之一 从 RDB 文件中读取数据写入目标端，因此，我们可以借鉴rdb文件解析功能
   项目地址：https://github.com/tair-opensource/RedisShake
   rdb.go源码地址：https://github.com/tair-opensource/RedisShake/tree/v4/internal/rdb

3）可以对照 redis 源码

```
  rdb.c 文件：https://github.com/redis/redis/blob/7.0-rc3/src/rdb.c       // RDB 文件读写，行1736：robj *rdbLoadObject
  rdb.h 文件：https://github.com/redis/redis/blob/unstable/src/rdb.h   // RDB version、RDB object types 等定义

```

注：  
   - rdb 对数字这一块的解码操作要特别注意，不一定能用 BitConverter.ToIntXX 来获得正确的值！！ 
   - redis7.x底层存储类型使用listpack替代ziplist。例如，若List大小超过阈值（list-max-listpack-size），Redis会切换为ziplist或quicklist编码


## 贡献

欢迎来自各界的贡献。对于重大变更，请先开一个 issue 来讨论你想要改变的内容。如果想共同维护此项目，请加我微信（Sd-LiYanJing）。

特别感兴趣的是：

 1. 随着redis版本变化，增加新类型的读解析支持
 2. 优化、改善代码，提升性能
 

## 交流群

添加微信（Sd-LiYanJing），备注GitHub-rdr，即可进群

注：如果本工具对您有所帮助，请动动发财的小手，给项目点个赞（点击 Star ），您的一票，是对我们最大的支持！


## License

This project is under Apache v2 License. See the [LICENSE](LICENSE) file for the full license text.