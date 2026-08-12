# Planexus 엔터프라이즈 관리자 가이드 (Admin & Operational Guide)

- **문서 버전**: v1.0.0-ENTERPRISE  
- **작성일자**: 2026년 8월 13일  
- **대상**: 시스템 관리자, Security/DevOps 엔지니어, 전사 기획/PMO 관리자  
- **문서 개요**: Planexus 4대 환경변수 부트스트랩, Keycloak OIDC SSO, ENCRYPTION_KEY 시크릿 암호화, 거버닝 AI 정책 및 감사 로그 운영  

---

## 1. 시스템 부트스트랩 및 필수 환경변수 (Bootstrap Specification)

Planexus 컨테이너 프로세스는 오직 **4개의 애플리케이션 환경변수**만으로 최소 인프라 구축 및 부트스트랩을 완수합니다.

```bash
# Planexus 실행 환경변수 명세
POSTGRES_DSN=postgres://planexus:Secr3tPass@10.10.60.5:5432/planexus?sslmode=disable
BOOTSTRAP_ADMIN=admin
BOOTSTRAP_ADMIN_PASSWORD=SuperSecretAdminPassword123!
ENCRYPTION_KEY=Base64Encoded32ByteKeyHere==
```

> **비상 관리자(Break Glass) 및 암호화 마스터 키**:  
> `BOOTSTRAP_ADMIN` 계정은 삭제가 불가능한 비상 복구 계정이며, `ENCRYPTION_KEY`는 Base64로 인코딩된 32바이트 암호화 키입니다 (`openssl rand -base64 32`).

---

## 2. 데이터 백업 및 시크릿 암호화 (`ENCRYPTION_KEY`)

```bash
docker run -d \
  --name planexus \
  -p 8080:8080 \
  -e POSTGRES_DSN="postgres://planexus:password@postgres:5432/planexus" \
  -e BOOTSTRAP_ADMIN="admin" \
  -e BOOTSTRAP_ADMIN_PASSWORD="change-this-strong-password" \
  -e ENCRYPTION_KEY="Base64Encoded32ByteKeyHere==" \
  planexus:v1.0.0
```

### 2.1 시크릿 복호화 보안 유지
`ENCRYPTION_KEY` 환경변수는 DB 내에 암호화 보관된 Keycloak Client Secret, 사내 AI Provider Token 및 개인 API/MCP 시크릿을 복호화(AES-256-GCM)하는 마스터 키로 정기 백업 및 보안 관리가 필수적입니다.

---

## 3. Keycloak OIDC SSO 및 RBAC 그룹 매핑

- **OIDC Discovery**: Keycloak Discovery 엔드포인트를 등록하고 Authorization Code + PKCE (S256) 인증을 켭니다.
- **Valid Redirect URI**: `https://planexus.internal/api/v1/auth/oidc/callback`
- **그룹 매핑**: Keycloak `/planexus-admins`, `/planexus-planners` 그룹을 사내 권한 그룹으로 맵핑하여 자동 RBAC 부여.

---

## 4. 거버닝 AI 및 무결성 감사 로그 (Audit Trail)

- **거버닝 AI (Governed AI)**: 사내 방화벽 내부 프라이빗 LLM 엔드포인트 맵핑 및 민감 전략 필터링 적용.
- **감사 로그 (Audit Trail)**: 전략 목표 수정, 수지 시뮬레이션 승인, OIDC 변경 등 모든 액션이 사용자 ID 및 IP 주소와 함께 무결성 감사 레코드로 영구 기록됩니다.
