export type User = {
  id: string; username: string; displayName: string; email?: string; title?: string
  organizationId?: string; roles: string[]; permissions: string[]
  mustChangePassword: boolean; csrfToken?: string; authMethod: string
}
export type Version = { service: string; version: string }
export type AuthConfig = { localEnabled: boolean; oidcEnabled: boolean; oidcLabel?: string }
export type Dashboard = { strategyCount: number; kpiHealth: number; projectCount: number; highRiskProjects: number; budgetTotal: number; actualCost: number; pendingPlans: number; decisionCount: number }
export type PersonalRecommendation = { type: string; id: string; title: string; reason: string; priority: number; severity: string; path: string }
export type PersonalDashboard = { ownedKpis: number; ownedProjects: number; myPlans: number; pendingApprovals: number; activeKeys: number; recommendations: PersonalRecommendation[] }
export type Strategy = { id: string; parentId?: string; name: string; kind: string; description: string; version: number; status: string; classification: string }
export type KPI = { id: string; strategyId?: string; parentId?: string; code: string; name: string; description: string; unit: string; frequency: string; target: number; actual: number; achievement: number; weight: number; source: string; classification: string }
export type Project = { id: string; strategyId?: string; name: string; description: string; status: string; progress: number; risk: string; budget: number; actualCost: number; classification: string }
export type Plan = { id: string; title: string; period: string; status: string; version: number; classification: string; content: Record<string, unknown> }
export type Decision = { id: string; title: string; decisionDate?: string; decision: string; reason: string; classification: string }
export type Intelligence = { id: string; category: string; title: string; sourceName: string; sourceUrl: string; publishedAt?: string; summary: string; importance: number; companyRelevance: string; potentialImpact: string; risk: string; opportunity: string; recommendedAction: string; classification: string }
export type Scenario = { id: string; name: string; description: string; status: string; assumptions: Record<string, unknown>; results?: Record<string, unknown>; classification: string; updatedAt: string }
export type AIAnswer = { answer: string; confidence: number; evidence: unknown; source: string[]; generatedAt: string; model: string; interactionId: string }
