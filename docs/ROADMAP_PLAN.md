# Planexus 엔터프라이즈 중장기 기술 로드맵 (Product Roadmap Plan)

- **문서 버전**: v1.0.0 ~ v3.0-VISION  
- **작성일자**: 2026년 8월 13일  
- **문서 분류**: 비즈니스 및 아키텍처 중장기 로드맵 (Strategic Product Roadmap)  

---

## 1. 비전 및 발전 마일스톤 개요

Planexus 플랫폼은 사내 오프라인 전략-KPI 통섭 및 시나리오 수지 시뮬레이션을 시작으로, 사내 AI 데이터 에이전트와 대화형으로 전사 전략 예측 및 자율 사업계획 조율을 수행하는 차세대 Autonomous Strategic Intelligence Platform으로 진화합니다.

```
==================================================================================================
                                [Planexus 단계별 마일스톤 아키텍처]
==================================================================================================
 [Phase 1: v1.0.0] (완료) ➔ Strategy-KPI Unification, Scenario Simulation, Governed AI, 8+ MCP
 [Phase 2: v1.5.0] (진행) ➔ Multi-Business Unit Financial Consolidation & ERP Real-time Sync
 [Phase 3: v2.0.0] (2026 Q4) ➔ AI Autonomous Strategy Prediction Copilot (NL-to-Strategy MCP 2.0)
 [Phase 4: v3.0.0] (2027)    ➔ Predictive Enterprise Resource Allocation & Autonomous Governance
==================================================================================================
```

---

## 2. Phase별 세부 기술 명세 및 추진 전략

### 2.1 Phase 1: v1.0.0 오프라인 전략 인텔리전스 OS 구축 (완료)
- **전략-KPI 통섭 & 시뮬레이션**: 비전-전략목표-KPI-포트폴리오 다차원 맵핑, 결정론적 수지 시뮬레이션.
- **Excel 임포트 & 롤백**: XLSX 일괄 가져오기 및 버전 무결성 롤백 엔진.
- **ENCRYPTION_KEY & Keycloak OIDC**: Base64 32바이트 키 기반 시크릿 AES-256 암호화, PKCE SSO 및 4대 환경변수 부트스트랩.
- **Streamable HTTP MCP**: AI 에이전트를 위한 8개 이상의 ACL-aware MCP Tools 탑재.

### 2.2 Phase 2: v1.5.0 멀티 사업부 재무 연결 & ERP 실시간 동기화 (2026 Q3)
- **다중 계열사/사업부 통합 재무 뷰**: 그룹사 및 자회사 전사 수지 실시간 연결.
- **SAP/Oracle ERP 파이프라인**: 사내 ERP 실제 집행 예산 데이터 실시간 바인딩.

### 2.3 Phase 3: v2.0.0 AI 자율 전략 예측 코파일럿 (2026 Q4)
- **NL-to-Strategy Action (MCP 2.0)**: AI 에이전트에 "올해 3분기 환율 변동 시나리오 수지 재계산하고 PMO 리스크 보고서 작성해줘" 요청 시 권한 검증 후 자율 수행.

---

## 3. 리소스 및 위험 관리 (Risk Matrix)

| 위험 요소 | 영향도 | 발생 가능성 | 대응 및 완화 전략 |
| :--- | :--- | :--- | :--- |
| **PostgreSQL DB 장애** | High | Low | Multi-AZ HA 클러스터 및 Read-Replica 구축 |
| **ENCRYPTION_KEY 키 손실** | High | Low | Base64 암호화 키 안전한 이중화 백업 보관 |
| **시나리오 변수 오설정** | Medium | Medium | 수지 시뮬레이션 승인 이력 관리 및 원클릭 롤백 레이어 제공 |
