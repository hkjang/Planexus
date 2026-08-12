package server

import (
	"net/http"
	"strings"
)

func (s *Server) openAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	_, _ = w.Write([]byte(strings.ReplaceAll(openAPISpec, "${VERSION}", s.version)))
}

const openAPISpec = `openapi: 3.0.3
info:
  title: Planexus Open API
  version: ${VERSION}
  description: |
    Enterprise Strategy and Planning Intelligence Platform API.
    Browser sessions require the X-CSRF-Token returned by login on unsafe methods.
    Personal Bearer keys are scope-limited and do not use CSRF.
servers:
  - url: /api/v1
security:
  - cookieAuth: []
  - bearerAuth: []
tags:
  - {name: Authentication}
  - {name: Dashboard}
  - {name: Strategy}
  - {name: Planning}
  - {name: Intelligence}
  - {name: Integration}
  - {name: Administration}
paths:
  /version:
    get:
      security: []
      tags: [Authentication]
      summary: Service build version
      responses: {'200': {$ref: '#/components/responses/Success'}}
  /auth/config:
    get:
      security: []
      tags: [Authentication]
      summary: Enabled login methods
      responses: {'200': {$ref: '#/components/responses/Success'}}
  /auth/login:
    post:
      security: []
      tags: [Authentication]
      summary: Bootstrap or local login
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [username, password]
              properties:
                username: {type: string}
                password: {type: string, format: password}
      responses:
        '200': {$ref: '#/components/responses/Success'}
        '401': {$ref: '#/components/responses/Error'}
        '429': {$ref: '#/components/responses/Error'}
  /auth/me:
    get:
      tags: [Authentication]
      summary: Current identity, roles, permissions and CSRF token
      responses: {'200': {$ref: '#/components/responses/Success'}}
  /auth/password:
    put:
      security: [{cookieAuth: []}]
      tags: [Authentication]
      summary: Change local password
      parameters: [{$ref: '#/components/parameters/CSRF'}]
      responses: {'204': {description: Password changed}}
  /auth/logout:
    post:
      tags: [Authentication]
      summary: Revoke current session
      parameters: [{$ref: '#/components/parameters/CSRF'}]
      responses: {'204': {description: Logged out}}
  /search:
    get:
      tags: [Dashboard]
      summary: Permission-filtered global search
      parameters:
        - {name: q, in: query, required: true, schema: {type: string, minLength: 2}}
      responses: {'200': {$ref: '#/components/responses/Success'}}
  /dashboard/executive:
    get:
      tags: [Dashboard]
      summary: Executive cockpit indicators
      responses: {'200': {$ref: '#/components/responses/Success'}}
  /dashboard/personal:
    get:
      tags: [Dashboard]
      summary: Personal workload indicators
      responses: {'200': {$ref: '#/components/responses/Success'}}
  /strategies:
    get:
      tags: [Strategy]
      summary: List authorized strategies
      responses: {'200': {$ref: '#/components/responses/Success'}}
    post:
      tags: [Strategy]
      summary: Create strategy
      parameters: [{$ref: '#/components/parameters/CSRF'}]
      responses: {'201': {$ref: '#/components/responses/Created'}}
  /kpis:
    get:
      tags: [Strategy]
      summary: List authorized KPIs and achievement
      responses: {'200': {$ref: '#/components/responses/Success'}}
    post:
      tags: [Strategy]
      summary: Create KPI
      parameters: [{$ref: '#/components/parameters/CSRF'}]
      responses: {'201': {$ref: '#/components/responses/Created'}}
  /projects:
    get:
      tags: [Strategy]
      summary: List authorized portfolio projects
      responses: {'200': {$ref: '#/components/responses/Success'}}
    post:
      tags: [Strategy]
      summary: Create portfolio project
      parameters: [{$ref: '#/components/parameters/CSRF'}]
      responses: {'201': {$ref: '#/components/responses/Created'}}
  /plans:
    get:
      tags: [Planning]
      summary: List accessible business plans
      responses: {'200': {$ref: '#/components/responses/Success'}}
    post:
      tags: [Planning]
      summary: Create business plan
      parameters: [{$ref: '#/components/parameters/CSRF'}]
      responses: {'201': {$ref: '#/components/responses/Created'}}
  /plans/{id}/submit:
    post:
      tags: [Planning]
      summary: Submit or immediately confirm according to workflow policy
      parameters:
        - {$ref: '#/components/parameters/ID'}
        - {$ref: '#/components/parameters/CSRF'}
      responses: {'200': {$ref: '#/components/responses/Success'}}
  /workflow/tasks:
    get:
      tags: [Planning]
      summary: List review tasks visible to current user
      responses: {'200': {$ref: '#/components/responses/Success'}}
  /workflow/tasks/{id}/action:
    post:
      tags: [Planning]
      summary: Approve or reject a workflow task
      parameters:
        - {$ref: '#/components/parameters/ID'}
        - {$ref: '#/components/parameters/CSRF'}
      responses: {'200': {$ref: '#/components/responses/Success'}}
  /decisions:
    get:
      tags: [Intelligence]
      summary: List authorized decision history
      responses: {'200': {$ref: '#/components/responses/Success'}}
    post:
      tags: [Intelligence]
      summary: Create decision record
      parameters: [{$ref: '#/components/parameters/CSRF'}]
      responses: {'201': {$ref: '#/components/responses/Created'}}
  /intelligence:
    get:
      tags: [Intelligence]
      summary: Search authorized intelligence
      parameters:
        - {name: category, in: query, schema: {type: string}}
        - {name: q, in: query, schema: {type: string}}
      responses: {'200': {$ref: '#/components/responses/Success'}}
    post:
      tags: [Intelligence]
      summary: Create intelligence item
      parameters: [{$ref: '#/components/parameters/CSRF'}]
      responses: {'201': {$ref: '#/components/responses/Created'}}
  /scenarios:
    get:
      tags: [Intelligence]
      summary: List authorized scenarios
      responses: {'200': {$ref: '#/components/responses/Success'}}
    post:
      tags: [Intelligence]
      summary: Create scenario
      parameters: [{$ref: '#/components/parameters/CSRF'}]
      responses: {'201': {$ref: '#/components/responses/Created'}}
  /scenarios/{id}/run:
    post:
      tags: [Intelligence]
      summary: Run deterministic portfolio impact simulation
      parameters:
        - {$ref: '#/components/parameters/ID'}
        - {$ref: '#/components/parameters/CSRF'}
      responses: {'200': {$ref: '#/components/responses/Success'}}
  /ai/query:
    post:
      tags: [Intelligence]
      summary: Query governed AI Gateway with permission-filtered evidence
      parameters: [{$ref: '#/components/parameters/CSRF'}]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [query]
              properties:
                query: {type: string, maxLength: 8000}
                useCase: {type: string, default: general}
      responses:
        '200': {$ref: '#/components/responses/Success'}
        '400': {$ref: '#/components/responses/Error'}
        '429': {$ref: '#/components/responses/Error'}
  /import/templates/{entityType}:
    get:
      tags: [Integration]
      summary: Download Excel import template
      parameters:
        - {name: entityType, in: path, required: true, schema: {type: string, enum: [strategy, kpi, project]}}
        - {name: sample, in: query, schema: {type: boolean}}
      responses: {'200': {description: XLSX template}}
  /import/preview:
    post:
      tags: [Integration]
      summary: Validate and preview Excel import
      parameters: [{$ref: '#/components/parameters/CSRF'}]
      responses: {'200': {$ref: '#/components/responses/Success'}}
  /import/{id}/commit:
    post:
      tags: [Integration]
      summary: Atomically commit validated import
      parameters:
        - {$ref: '#/components/parameters/ID'}
        - {$ref: '#/components/parameters/CSRF'}
      responses: {'200': {$ref: '#/components/responses/Success'}}
  /import/{id}/rollback:
    post:
      tags: [Integration]
      summary: Roll back records created by an import
      parameters:
        - {$ref: '#/components/parameters/ID'}
        - {$ref: '#/components/parameters/CSRF'}
      responses: {'200': {$ref: '#/components/responses/Success'}}
  /import/history:
    get:
      tags: [Integration]
      summary: List import jobs
      responses: {'200': {$ref: '#/components/responses/Success'}}
  /notification-rules:
    get:
      tags: [Integration]
      summary: List personal and administrable notification rules
      responses: {'200': {$ref: '#/components/responses/Success'}}
    post:
      tags: [Integration]
      summary: Create notification rule
      parameters: [{$ref: '#/components/parameters/CSRF'}]
      responses: {'201': {$ref: '#/components/responses/Created'}}
  /notification-rules/{id}:
    delete:
      tags: [Integration]
      summary: Delete owned or administrable notification rule
      parameters:
        - {$ref: '#/components/parameters/ID'}
        - {$ref: '#/components/parameters/CSRF'}
      responses: {'204': {description: Deleted}}
  /notifications:
    get:
      tags: [Integration]
      summary: List current user's notifications
      responses: {'200': {$ref: '#/components/responses/Success'}}
  /notifications/{id}/read:
    post:
      tags: [Integration]
      summary: Mark notification read
      parameters:
        - {$ref: '#/components/parameters/ID'}
        - {$ref: '#/components/parameters/CSRF'}
      responses: {'204': {description: Marked read}}
  /keys:
    get:
      security: [{cookieAuth: []}]
      tags: [Authentication]
      summary: List current user's API and MCP key metadata
      responses: {'200': {$ref: '#/components/responses/Success'}}
    post:
      security: [{cookieAuth: []}]
      tags: [Authentication]
      summary: Issue scoped personal key; secret is shown once
      parameters: [{$ref: '#/components/parameters/CSRF'}]
      responses: {'201': {$ref: '#/components/responses/Created'}}
  /keys/policy:
    get:
      security: [{cookieAuth: []}]
      tags: [Authentication]
      summary: Get effective personal key lifetime, rotation and allowed scopes
      responses: {'200': {$ref: '#/components/responses/Success'}}
  /keys/{id}/rotate:
    post:
      security: [{cookieAuth: []}]
      tags: [Authentication]
      summary: Rotate personal key with policy-controlled overlap
      parameters:
        - {$ref: '#/components/parameters/ID'}
        - {$ref: '#/components/parameters/CSRF'}
      responses: {'201': {$ref: '#/components/responses/Created'}}
  /keys/{id}:
    delete:
      security: [{cookieAuth: []}]
      tags: [Authentication]
      summary: Revoke personal key
      parameters:
        - {$ref: '#/components/parameters/ID'}
        - {$ref: '#/components/parameters/CSRF'}
      responses: {'204': {description: Revoked}}
  /admin/settings:
    get:
      tags: [Administration]
      summary: List versioned settings; secret values are never returned
      responses: {'200': {$ref: '#/components/responses/Success'}}
  /admin/settings/{category}/{key}:
    put:
      tags: [Administration]
      summary: Create or update plain or encrypted setting
      parameters:
        - {name: category, in: path, required: true, schema: {type: string}}
        - {name: key, in: path, required: true, schema: {type: string}}
        - {$ref: '#/components/parameters/CSRF'}
      responses: {'204': {description: Saved}}
    delete:
      tags: [Administration]
      summary: Delete setting
      parameters:
        - {name: category, in: path, required: true, schema: {type: string}}
        - {name: key, in: path, required: true, schema: {type: string}}
        - {$ref: '#/components/parameters/CSRF'}
      responses: {'204': {description: Deleted}}
  /admin/authentication/oidc:
    get:
      tags: [Administration]
      summary: Get redacted Keycloak/OIDC configuration
      responses: {'200': {$ref: '#/components/responses/Success'}}
    put:
      tags: [Administration]
      summary: Save encrypted Keycloak/OIDC configuration and mappings
      parameters: [{$ref: '#/components/parameters/CSRF'}]
      responses: {'204': {description: Saved}}
  /admin/ai:
    get:
      tags: [Administration]
      summary: Get redacted governed AI configuration
      responses: {'200': {$ref: '#/components/responses/Success'}}
    put:
      tags: [Administration]
      summary: Save encrypted AI Gateway and model policy
      parameters: [{$ref: '#/components/parameters/CSRF'}]
      responses: {'204': {description: Saved}}
  /admin/ai/usage:
    get:
      tags: [Administration]
      summary: AI usage and latency aggregates
      responses: {'200': {$ref: '#/components/responses/Success'}}
  /admin/workflows:
    get:
      tags: [Administration]
      summary: List workflow policies
      responses: {'200': {$ref: '#/components/responses/Success'}}
  /admin/workflows/{resourceType}:
    put:
      tags: [Administration]
      summary: Enable, disable or replace workflow steps
      parameters:
        - {name: resourceType, in: path, required: true, schema: {type: string}}
        - {$ref: '#/components/parameters/CSRF'}
      responses: {'204': {description: Saved}}
  /admin/users:
    get:
      tags: [Administration]
      summary: List users and role assignments
      responses: {'200': {$ref: '#/components/responses/Success'}}
  /admin/users/{id}:
    put:
      tags: [Administration]
      summary: Update user profile, organization and active state
      parameters:
        - {$ref: '#/components/parameters/ID'}
        - {$ref: '#/components/parameters/CSRF'}
      responses: {'204': {description: Saved}}
  /admin/users/{id}/roles:
    post:
      tags: [Administration]
      summary: Replace direct role assignments
      parameters:
        - {$ref: '#/components/parameters/ID'}
        - {$ref: '#/components/parameters/CSRF'}
      responses: {'204': {description: Saved}}
  /admin/organizations:
    get:
      tags: [Administration]
      summary: List organization hierarchy
      responses: {'200': {$ref: '#/components/responses/Success'}}
    post:
      tags: [Administration]
      summary: Create organization
      parameters: [{$ref: '#/components/parameters/CSRF'}]
      responses: {'201': {$ref: '#/components/responses/Created'}}
  /admin/organizations/{id}:
    put:
      tags: [Administration]
      summary: Update organization with cycle protection
      parameters:
        - {$ref: '#/components/parameters/ID'}
        - {$ref: '#/components/parameters/CSRF'}
      responses: {'204': {description: Saved}}
  /admin/roles:
    get:
      tags: [Administration]
      summary: List editable RBAC permission policies
      responses: {'200': {$ref: '#/components/responses/Success'}}
  /admin/roles/{id}:
    put:
      tags: [Administration]
      summary: Replace role permissions
      parameters:
        - {name: id, in: path, required: true, schema: {type: string}}
        - {$ref: '#/components/parameters/CSRF'}
      responses: {'204': {description: Saved}}
  /admin/audit:
    get:
      tags: [Administration]
      summary: List append-only audit events
      responses: {'200': {$ref: '#/components/responses/Success'}}
  /admin/health:
    get:
      tags: [Administration]
      summary: Database, Go runtime and AI health
      responses: {'200': {$ref: '#/components/responses/Success'}}
  /admin/backups:
    get:
      tags: [Administration]
      summary: List backup, validation and restore jobs
      responses: {'200': {$ref: '#/components/responses/Success'}}
  /admin/backups/export:
    get:
      tags: [Administration]
      summary: Download complete logical .plxbackup archive
      responses: {'200': {description: Planexus backup archive}}
  /admin/backups/validate:
    post:
      tags: [Administration]
      summary: Validate backup schema, checksum and encryption key fingerprint
      parameters: [{$ref: '#/components/parameters/CSRF'}]
      responses: {'200': {$ref: '#/components/responses/Success'}}
  /admin/backups/restore:
    post:
      tags: [Administration]
      summary: Transactionally restore a validated backup
      parameters: [{$ref: '#/components/parameters/CSRF'}]
      responses: {'200': {$ref: '#/components/responses/Success'}}
components:
  parameters:
    ID:
      name: id
      in: path
      required: true
      schema: {type: string, format: uuid}
    CSRF:
      name: X-CSRF-Token
      in: header
      required: false
      description: Required for unsafe browser-session requests; not used with Bearer keys.
      schema: {type: string}
  responses:
    Success:
      description: Successful response
      content:
        application/json:
          schema: {}
    Created:
      description: Resource created
      content:
        application/json:
          schema: {type: object, additionalProperties: true}
    Error:
      description: Error response
      content:
        application/json:
          schema:
            type: object
            properties:
              error: {type: string}
              message: {type: string}
  securitySchemes:
    cookieAuth:
      type: apiKey
      in: cookie
      name: planexus_session
    bearerAuth:
      type: http
      scheme: bearer
      description: Scoped Planexus personal API/MCP key
`
