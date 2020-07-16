## 4种 Log 级别
- Error
- Warn
- Info
- Debug

## 2种写入方式

### 同步写log 

```
log := logs.NewLogger(10000)
log.Info("%s:%d","info",20)
log.Warn("warning")
log.Debug("debug")
```

### 异步写log

```
log := logs.NewLogger(10000)
log.Async()
log.Info("%s:%d","info",20)
log.Warn("warning")
log.Debug("debug")
```

### 结束logger 

```
log.Close()
```


### log 接口，目前只支持 console 

```
log := logs.NewLogger(10000)
log.SetLogger("console", "")
```
