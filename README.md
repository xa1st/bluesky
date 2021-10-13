skyblue
=====
天朗物业通知获取工具 golang版本, 之所以叫这个名字，是因为他APP包名就是这个...

> 有关该项目的说明详见 https://del.pub/skyblue

![预览图](https://raw.githubusercontent.com/mopo/bluesky/master/preview.png)

## 1. 接口来源

* 天朗物业官方APP "天朗蔚蓝生活APP"
> 应用宝和APPSTORE直接搜名字即可

## 2. 使用
```bash
git clone git@github.com:mopo/skyblue.git
cd skyblue
vim config.ini   // 打开邮件配置填好邮件smtp
go mod tidy
go run main.go
```

## 3. 其它
> 发邮件时请设好间隔，各大厂商有邮件发送限制

## 4. license
Licensed under MIT.
