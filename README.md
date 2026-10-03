# 电路板设计工作台 (PCB Workbench)

第一版: 板框、焊盘、铜线、过孔与网络关系编辑; 人工连线可保存、撤销,
并按真实铜皮几何检查连通与短路。自动布线留待后续版本。

第二版: 装配方案 ( stuffing variants ) — 同一块板可声明多个装配方案,
可选跳线与未安装器件按方案参与或退出导电路径; 铜线与过孔几何始终
共用同一份依据。可一键比较所有方案, 报告只在某些方案中短接/断开的
网络, 并逐段回看该方案下经由已装器件形成的路径。板资料局部缺失时
保留其余可用内容并逐项指出位置, 不再让整个画布失去响应。

## 结构

- `backend/` — Go 后端 (仅标准库): 几何引擎、连通性检查、持久化、HTTP API
- `frontend/` — React + TypeScript + zustand + Vite: SVG 画布编辑器

## 运行

```bash
cd frontend && npm run dev        # 开发模式 (代理 /api 到 :8080)
# 或: npm run build 后由后端直接托管
cd backend && go run .            # http://localhost:8080
```

## 测试

```bash
cd backend && go test ./...
```

## 设计要点

- **坐标**: 全部为整数纳米 (nm) 的板级坐标; 界面缩放只是视图变换,
  几何判断与缩放无关 (`TestZoomIndependence`)。
- **连通性按几何而非网络名**: 元素展开为各层铜皮形状 (圆/矩形/圆头线段),
  同层相交才合并; 过孔按声明层 (`via.layers`) 桥接, 通孔焊盘存在于所有层。
  跨层交叉不相连 (`TestCrossLayerCrossingNotConnected`)。
- **错误定位与路径回放**: 短路/开路均返回逐段导电路径 (`errors[].steps`),
  每段标明元件 (焊盘/铜线/过孔)、所属网络 (未命名铜皮显示为
  “(未命名铜皮)”)、所在层与接触/间隙坐标; 跨层段显式列出过孔桥接的
  层 (如 `top+bottom`)。前端点击故障可跳转放大, 并可在画布叠加高亮
  完整路径, 而不仅是两个网络名。
- **网络状态与即时复核**: `POST /api/board/check` 同时返回每网络状态
  (`netStatus`: connected/open/short、铜皮域数、短接对象)。前端在放置、
  局部移动、删除过孔/铜线、撤销重做及改层改网络后 200ms 自动复核,
  并与编辑前状态对比, 标注每个网络“已断开/新发生短路/已恢复连通”。
- **撤销/重做**: 前端保存板子快照历史 (Ctrl+Z / Ctrl+Y)。
- **装配方案**: `board.assemblies[]` 声明方案, `exclude` 列出该方案
  不安装的元件 ID。检查按 `?asm=<id>` 限定到具体方案; 比较接口对
  基板与每个方案独立分析同一份几何, 输出 `diffs` (只在部分方案中
  短接/断开的网络及其逐方案状态)。切换方案、移动过孔或撤销后, 前端
  按方案缓存并即时复核, 各方案结论互不沿用; 方案随板保存, 重开后
  仍可独立核对。
- **容错加载**: 前后端均对缺失字段做局部修复 (空数组、默认层、默认
  线宽、跳过无效元件), 每项修复都带板级坐标警告, 其余内容照常编辑。
- **保存/导出/导入**: `PUT /api/board` 持久化到 `backend/data/board.json`;
  导出/导入为完整 JSON, 元件 ID 原样保留 (`TestExportImportPreservesIdentity`)。
- **验证板**: 内置示例板含跨层交叉 (同网络名但不连通, 报开路) 与
  0.1 mm 细小间隙 (报开路并定位到间隙), `POST /api/board/reset` 可恢复。

## API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | /api/board | 当前板 |
| PUT | /api/board | 保存 (修订号自增) |
| POST | /api/board/check | 检查 (可带未保存的板子; `?asm=<id>` 限定装配方案) |
| POST | /api/board/assemblies/compare | 比较所有装配方案的连通结论 |
| GET | /api/board/export | 导出 JSON |
| POST | /api/board/import | 导入 JSON (保留 ID) |
| POST | /api/board/reset | 重置为验证板 |
