# BUPT-Hotel
波普特大酒店管理系统，北京邮电大学2024年春季-人工智能学院-软件工程课程设计。

# 技术栈

- Vue3
- Go backend:Gin + GORM

# News
- [2024-11-28]写完了登记入住/退房
- [2024-11-29凌晨]Doing，调度基本逻辑写完，等待测试
- [2024-11-29]完成了房间目标温度/风速的接口设计(ac_handler)，并完成调度器的相关测试，还有诸多小细节的改进，比如说logger等此外，在ApiFox写了一个测试样例(制冷)的自动化测试。

# 后端架构
```
backend/
├── api/
│   └── router.go
├── cmd/
│   ├── hotel.db
│   ├── launch.sh
│   ├── logs
│   │   └── 2024-11-30.log
│   └── main.go
├── docs/
├── go.mod
├── go.sum
└── internal/
    ├── db/
    │   ├── init.go
    │   ├── model.go
    │   └── room_repository.go
    ├── handlers/
    │   ├── ac_handler.go
    │   ├── common.go
    │   └── room_handler.go
    ├── logger/
    │   └── logger.go
    ├── scheduler/
    │   ├── old_scheduler
    │   ├── scheduler.go
    │   └── scheduler_test.go
    └── service/
        ├── monitor.go
        └── service.go
```
说明：
- /cmd - 项目的启动文件
- /internal - 私有的代码库
    - /db - 数据库相关
    - /handler - 接口设计相关
    - /scheduler - 调度器相关
    - /service - 服务层(监视器，调度器实际上应该放这里)
- /api - api的相关定义
- /docs - 文档所在

后端的架构大致如此，后续应该还会继续有部分更改。

# 如何启动
backend/cmd/有个launch.sh文件
```Bash
bash ./backend/cmd/launch.sh
```

# 如何运行自动化测试
对应的package下有以*_test.go结尾的文件，进入对应的目录
```Bash
#TestHandleDuplicateRequests为测试函数，-v选择logger模式
go test (-v) -run TestHandleDuplicateRequests
```

# 调度器逻辑

调度的核心策略在于
1. 优先级调度
2. 时间片轮转

我这里实现了一个monitorServiceStatus不断对服务队列和等待队列内的对象进行更新。

当有新的调度请求到来时：
1. 判断是否在两个队列中
2. 服务队列未满直接进入，已满则进入调度方法
3. 顺序进行优先级调度和时间片调度
4. 时间片调度中对于相同风速(优先级)，一个等待对象等待时间到达后，替换相同风速中服务时间最长的，若无则重置等待时间

基本逻辑差不多如此。