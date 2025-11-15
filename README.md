RDR: redis data reveal
=================================================

## About（介绍）

RDR(redis data reveal) is a tool to parse redis rdbfile.  Through redis memory analysis, it helps us find bigkeys and keys that are growing without limit.

This repository is fok  from github.com/xueqiu/rdr.  The requrie rdb file parse is github.com/dongmx/rdb ，now in this repository  has been replaced  github.com/919927181/rdb

- RDR(redis data Reveal)是一个用于离线解析 redis rdb 文件的工具，通过redis内存分析，帮助我们找出bigkey、日益无限制增长的key等。

- 本项目fok自github.com/caiqing0204/rdr <--fok from--xueqiu/rdr，该工具依赖 dongmx/rdb 来解析redis rdb 文件。在此，对以上开源作者，以及提供灵感、pr的朋友们表示感谢。

-  与redis-rdb-tools相比，RDR是由golang实现的，速度更快。

注：请阅读我的帖子 [《redis内存离线分析工具选型》](https://mp.weixin.qq.com/s/h7YJd0dVui0FqJLkG8RXxQ)


## Applicable Redis Version（适用 redis 版本）

 - RDR V1. 适用于 Redis6 版本 （redis 5.x ~ 6.x ，rdb 的版本则是 9 。
 - Redis 7.0 以上版本（rdb 的版本是 10 +），如果本工具出错：
 
   - 可以尝试通过 RedisShake 数据迁移工具，将redis7 RDB数据迁移到redis6下，然后再用rdr\进行分析。
   - 可以使用 golang版本的工具：github.com/HDT3213/rdb  -->  github.com/919927181/rdr-guigo-Wails【桌面GUI程序】 

备注：

  - 已知问题： list的元素个数是个超大数字（可能溢出）。
  - 本工具的核心依赖是 rdb 文件解析，不同版本的 redis，其 rdb 文件存在差异，也会增加新的数据类型，存在数据兼容性问题。
  - RDR 判断 redis rdb 版本的涉及两个文件：
    - \decoder\decoder.go 中的 'if d.rdbVer <' 我已改成小于16
    - 依赖\vendor\github.com\919927181\rdb\decoder.go 中的变量rdbVersion,我已改成20。


## Change（变更）
- caiqing0204：在源代码基础上，增加了key所属DB，这样可以更直观的查看key元信息。
- 泰山李工：
   - v1.0.2
     - 将依赖 github.com/dongmx/rdb 中的rdbVersion 由9改成20【2025-11-08】
     - 修改html布局、将标题英文改为中文 【2025-11-08】
	 
   - v1.0.3 
     - 升级chartjs版本，实现图表tip时，显示更人性化的数字【2025-11-13】
     - 将2021年3月 至 2023年7月，在原作者 github.com/xueqiu/rdr/pulls，除过滤小key外的其他pulls，均同步过来、并解决完毕。
	 - 遗留问题： redis3.2+新引入的encoding为quicklist作为list的基础类型，list的元素个数是个超大数字，在分析时可能溢出
	 

备注：github.com/dongmx/rdb  该项目已无维护。本着不重复造轮子的思想，将创建一个基于github.com/hdt3213/rdb的redis 离线内存分析工具 rdr2 【待创建】。


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
![show example](https://yqfile.alicdn.com/img_9bc93fc3a6b976fdf862c8314e34f454.png)

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

## develop (rdr 开发）

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

3. 运行
   
```
 1.如果改动了静态资源（css\js\html），需要使用go-bindata将静态资源文件嵌入到go文件里

 //go-bindata -prefix "static/" -o=static/static.go -pkg=static -ignore static.go static/...
 
 //go-bindata -prefix "views/" -o=views/views.go -pkg=views -ignore views.go views/...
 
 2. 作用是在编译前自动化生成某类代码;它常用于自动生成代码
 
 go generate
 ```



4. 构建，输出linu下的可执行文件【见上面】


## develop  （rdb 文件解析开发）

大部分 rdb 文件的解析应该都是按照下面这个文档来的：https://github.com/sripathikrishnan/redis-rdb-tools/wiki/Redis-RDB-Dump-File-Format

作为开发者，可以对照 redis 源码里面的 rdb.c 这个文件：https://github.com/redis/redis/blob/7.0-rc3/src/rdb.c

注：rdb 对数字这一块的解码操作要特别注意，不一定能用 BitConverter.ToIntXX 来获得正确的值！！

## License

This project is under Apache v2 License. See the [LICENSE](LICENSE) file for the full license text.
