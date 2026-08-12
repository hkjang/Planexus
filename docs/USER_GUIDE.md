# Planexus 엔터프라이즈 사용자 가이드 (User Guide & Manual)

- **문서 버전**: v1.0.0-ENTERPRISE  
- **작성일자**: 2026년 8월 13일  
- **대상**: 일반 임직원, 전략기획자/PMO, 사업부 수지 작성자, AI MCP 클라이언트 사용자  
- **문서 개요**: 전략-KPI 맵핑, 시나리오 수지 시뮬레이션, Excel 임포트/롤백, Personal Key 발급 및 Streamable MCP 연동 매뉴얼  

---

## 1. 개요 및 비즈니스 워크플로우 (Strategy Workflow)

Planexus는 비전-전략-KPI 목표, 프로젝트 포트폴리오, 시나리오 수지 계획 및 거버닝 AI 통섭을 완벽히 지원하는 오프라인 전사 전략 OS입니다.

---

## 2. 전략목표 ➔ KPI ➔ 프로젝트 포트폴리오 다차원 맵핑

- **전략 트리(Strategy Tree)**: 전사 비전 아래 주력 전략 목표(Pillar)를 등록하고 조직 단위별 KPI target을 다차원으로 구성합니다.
- **포트폴리오 추적**: 프로젝트 예산 및 이행률을 KPI 목표 달성 지표와 직관적으로 연동합니다.

---

## 3. 시나리오 수지 시뮬레이션 및 Excel 임포트/롤백

### 3.1 결정론적 수지 시뮬레이션
- 시장 변수, 환율, 원가 인상율을 조정하여 **Optimistic(낙관), Base(기본), Pessimistic(비관)** 3대 시나리오 수지를 원클릭으로 재계산합니다.
- 수지 시나리오 변경 후 결재 승인 워크플로우를 통해 승인권자 최종 승인을 받습니다.

### 3.2 Excel/XLSX 일괄 업로드 및 원클릭 롤백
- 기존 사업계획 엑셀 파일을 일괄 임포트하여 데이터 구조체로 즉시 변환합니다.
- 임포트 오류 발생 시 **[원클릭 버전 롤백]** 기능을 실행하여 이전 수지 상태로 안전하게 복원합니다.

---

## 4. Personal API / MCP Key 발급 및 AI 연동

1. 프로필 메뉴 ➔ **`/me/keys` (개인 API/MCP 키)** 이동.
2. **[신규 Personal Key 발급]** 클릭 ➔ `pln_7f9c8d11a2b3c4d5_xxxxxxxx` 형식 키 생성.
3. Claude Desktop 또는 Cursor 설정 파일에 MCP 서버를 등록하여 자연어로 전사 KPI 현황 조회:

```json
{
  "mcpServers": {
    "planexus": {
      "command": "curl",
      "args": [
        "-X", "POST",
        "-H", "Authorization: Bearer pln_7f9c8d11a2b3c4d5_xxxxxxxx",
        "https://planexus.internal/mcp"
      ]
    }
  }
}
```

### 제공되는 핵심 MCP Tools 목록
1. `planexus_search_kpis`: 전사 KPI 목표 및 실적 달성률 분석
2. `planexus_get_portfolio_status`: 프로젝트 포트폴리오 집행 예산 및 리스크 조회
3. `planexus_run_scenario_simulation`: 시나리오 변수(환율/원가) 시뮬레이션 재계산
4. `planexus_list_governance_issues`: 미승인 수지 계획 및 위험 이탈 KPI 알림 리포트
