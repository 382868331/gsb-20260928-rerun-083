# 电路板设计工作台 (PCB Workbench)

第一版: 板框、焊盘、铜线、过孔与网络关系编辑; 人工连线可保存、撤销,
并按真实铜皮几何检查连通与短路。自动布线留待后续版本。

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
- **装配方案 (variants)**: 同一块板可声明多个装配方案
  (`board.variants`), 每个方案只记录自己的元件选择 —— 未安装器件
  (`dnp`, 按焊盘 id) 与已安装跳线 (`jumpers`, 按跳线 id)。
  铜线与过孔几何在所有方案间共享同一份依据, 不复制。跳线
  (`board.jumpers`) 是可选导电连接, 只有在方案中安装时才参与导通。
  `POST /api/board/check` 接受 `{board, variantId}`, 每次调用都从
  提交的板修订重新分析, 切换方案、移动过孔或撤销后不会沿用上一方案
  的结论; `POST /api/board/variants/compare` 对基准视图与全部方案
  逐一复核, 报告哪些网络只在某个方案中短接或断开 (`diff` 的
  `onlyOpen`/`onlyShort`), 故障路径仍逐段列出经由的元件
  (含跳线 id)。方案随板一起保存, 重开后可独立核对。
- **局部缺失容忍**: 板资料缺少某些字段 (如 `pads`/`layers`/
  `variants`) 或个别元件坐标非法时, 前端 `normalizeBoard` 与后端
  `sanitizeBoard` 保留其余内容, 按空列表/默认层兜底, 并在状态栏
  指出缺失位置 (如 `焊盘[2] (pad-x) 坐标缺失, 已跳过`),
  不让整个画布失去响应。
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
- **方案比较与冻结依据**: 比较报告绑定所用板修订与每个方案的装配
  选择 (dnp/jumpers); 报告打开后移动过孔、改跳线或撤销都会被识别为
  依据过期, 可保留冻结结果或对当前板重新比较, 不会悄悄替换依据。
  报告及其依据 (板快照 + 几何/装配指纹) 随工程一起持久化
  (`board.json` 的 `compares`), 保存重开、撤销后重开都能恢复;
  指纹不含修订号, 撤销回相同几何再保存仍识别为“板未变化”。
  重新比较生成新报告并保留旧报告 (`GET /api/board/compares` 列出全部,
  并标注哪份依据与当前板一致); 旧报告的路径回放仍基于其冻结的旧修订,
  界面明确标注, 不会冒充当前几何。
  每个方案的结果区分 无故障 / 没有可用路径 (empty) / 资料缺失
  (incomplete, 如引用了不存在的器件或跳线), 单个方案异常不影响其他
  方案与当前板的检查、保存。只在某一方案中导通的网络可逐段查看其
  铜线/过孔/已装器件路径, 并标明路径来自当前几何还是旧修订。
- **保存/导出/导入**: `PUT /api/board` 持久化到 `backend/data/board.json`;
  导出/导入为完整 JSON, 元件 ID 原样保留 (`TestExportImportPreservesIdentity`)。
- **验证板**: 内置示例板含跨层交叉 (同网络名但不连通, 报开路) 与
  0.1 mm 细小间隙 (报开路并定位到间隙), `POST /api/board/reset` 可恢复。

## API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | /api/board | 当前板 |
| PUT | /api/board | 保存 (修订号自增) |
| POST | /api/board/check | 检查 (可带未保存的板子; 支持 {board, variantId} 按方案复核) |
| POST | /api/board/variants/compare | 比较全部装配方案, 报告方案间网络差异 |
| GET | /api/board/compares | 列出随工程保存的冻结比较报告 (含依据快照与是否匹配当前板) |
| POST | /api/board/netpath | 单网络在某方案中的逐段导通路径 ({board, variantId, netId}) |
| GET | /api/board/export | 导出 JSON |
| POST | /api/board/import | 导入 JSON (保留 ID) |
| POST | /api/board/reset | 重置为验证板 |
