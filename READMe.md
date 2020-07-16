## 同步写log 

```
log := logs.NewAppLogger(10000)
log.SetLogger("console", "")
log.Info("info")
log.Warn("warning")
log.Debug("debug")
```

## 异步写log

```
log := logs.NewAppLogger(10000)
log = log.Async()
log.Info("info")
log.Warn("warning")
log.Debug("debug")
```

## 结束logger 

```
log.Close()
```
