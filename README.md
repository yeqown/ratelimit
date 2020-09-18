## ratelimit
policies of ratelimit, including (bbr, token, leaky bucket .etc)

### Noun explanation

* `CPU (load)` Slip average of CPU load in last 1 second, and collect time interval is 250ms]
* `InFlight` The number of request those are dealing by the server.
* `Pass` The count of success count.
* `RT` Response time of success requests.
* `MaxPass` 表示最近 5s 内，单个采样窗口中最大的请求数
* `MinRt` 表示最近 5s 内，单个采样窗口中最小的响应时间
* `Windows` 表示一秒内采样窗口的数量，默认配置中是 5s 50 个采样，那么 windows 的值为 10

### Algorithm

`cpu > 800 AND (Now - PrevDrop) < 1s AND (MaxPass * MinRt * Windows / 1000) < InFlight`

### References

* https://github.com/go-kratos/kratos/blob/master/pkg/ratelimit/bbr/bbr.go
* https://mp.weixin.qq.com/s/ajfNMHY_LJsCnVlyoCT6qg **[Chinese POST]**
* https://github.com/alibaba/sentinel-golang/wiki/%E7%B3%BB%E7%BB%9F%E8%87%AA%E9%80%82%E5%BA%94%E6%B5%81%E6%8E%A7