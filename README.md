RDR: redis data reveal
=================================================

## About（用途）
RDR(redis data reveal) is a tool to parse redis rdbfile. Comparing to [redis-rdb-tools](https://github.com/sripathikrishnan/redis-rdb-tools), RDR is implemented by golang, much faster (5GB rdbfile takes about 2mins on my PC).

 - RDR(redis data Reveal)是一个解析redis rdb文件的工具，通过redis内存分析，找出bigkey、找出日益无限制增长的key。

 - 与redis-rdb-tools相比，RDR是由golang实现的，速度更快。

本源码fok自github.com/caiqing0204/rdr，他fok自github.com/xueqiu/rdr，在此对开源作者表示感谢。

注：请阅读我的帖子 [《redis内存离线分析工具选型》](https://mp.weixin.qq.com/s/h7YJd0dVui0FqJLkG8RXxQ)


问题：rdr格式在解析list格式有问题，list的元素个数是个超大数字（可能溢出），用HDT3213/rdb验证了下，没有解析出list中的数据
解决：可以尝试通过 RedisShake 数据迁移工具，将redis7 RDB数据迁移到redis6下，然后再用rdr\进行分析。


## change（变更）
- caiqing0204：在源代码基础上，增加了key所属DB，这样可以更直观的查看key元信息。
- 泰山李工：
   - v1.0.2
     - 将依赖 github.com/dongmx/rdb 中的rdbVersion 由9改成20【2025-11-08】
     - 修改html布局、将标题英文改为中文 【2025-11-08】
	 
   - v1.0.3 
     - 升级chartjs版本，实现图表tip时，显示更人性化的数字【2025-11-13】
     - 将2021年3月 至 2023年7月，在原作者 github.com/xueqiu/rdr/pulls，除过滤小key外的其他pulls，均同步过来、并解决完毕。
	 - 遗留问题： redis3.2+新引入的encoding为quicklist作为list的基础类型，list的元素个数是个超大数字，在分析时可能溢出
	 
   结束：
     - github.com/dongmx/rdb  该redis 数据文件离线分析项目多年不维护了，个人精力有限，因此转向使用比较活跃的github.com/hdt3213/rdb
	 - 即将创建新开源项目rdr2，敬请期待。

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

## develop (开发）

1. 如果你需要修改依赖，请将依赖fok到自己仓库下，然后源码引用自己的地址
   或 直接修改下载下来的依赖项目的源码

2. 如果你需要修改html

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

## License

This project is under Apache v2 License. See the [LICENSE](LICENSE) file for the full license text.
