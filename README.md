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
- **连通性按几何而非网络名**: 元素展开为各层铜皮形状 (圆/矩形/圆头线段),
  同层相交才合并; 过孔按声明层 (`via.layers`) 桥接, 通孔焊盘存在于所有层。
  跨层交叉不相连 (`TestCrossLayerCrossingNotConnected`)。
- **错误定位**: 短路报告两网络铜皮的接触点; 开路报告同网络两连通域间
  最近间隙的两端点, 前端点击错误可跳转放大到该位置。
- **逐段故障路径**: 短路/开路错误附带 `segments` 列表, 每一段给出两端
  元件、所在层与接触/断开点坐标及文字说明; 前端可逐段点击跳转,
  覆盖多过孔、未命名铜线、相邻异网络焊盘等场景。
- **即时复核**: 移动/删除过孔等局部编辑、撤销/重做后自动重新检查
  (提交未保存的板子), 状态栏明确哪些网络因此断开、短接或仍然连通;
  检查响应同时返回每个网络的状态 (`nets`: connected/open/short)。
- **撤销/重做**: 前端保存板子快照历史 (Ctrl+Z / Ctrl+Y)。
- **保存/导出/导入**: `PUT /api/board` 持久化到 `backend/data/board.json`;
  导出/导入为完整 JSON, 元件 ID 原样保留 (`TestExportImportPreservesIdentity`)。
- **验证板**: 内置示例板含跨层交叉 (同网络名但不连通, 报开路) 与
  0.1 mm 细小间隙 (报开路并定位到间隙), `POST /api/board/reset` 可恢复。

## API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | /api/board | 当前板 |
| PUT | /api/board | 保存 (修订号自增) |
| POST | /api/board/check | 检查 (可带未保存的板子), 返回 {errors, nets} |
| GET | /api/board/export | 导出 JSON |
| POST | /api/board/import | 导入 JSON (保留 ID) |
| POST | /api/board/reset | 重置为验证板 |
