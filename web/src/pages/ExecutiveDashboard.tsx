import { useEffect, useState } from 'react'
import { Alert, Box, Card, CardContent, Chip, Grid, LinearProgress, Skeleton, Stack, Typography } from '@mui/material'
import { AccountTreeOutlined, BusinessCenterOutlined, CrisisAlertOutlined, FlagOutlined, LightbulbOutlined, TrendingUpOutlined } from '@mui/icons-material'
import { get } from '../api/client'
import type { Dashboard } from '../types'
import { PageHeader } from '../components/PageHeader'
import { useAuth } from '../auth/AuthContext'

const currency = new Intl.NumberFormat('ko-KR', { notation: 'compact', style: 'currency', currency: 'KRW', maximumFractionDigits: 1 })
export function ExecutiveDashboard() {
  const { user } = useAuth(); const [data, setData] = useState<Dashboard | null>(null); const [error, setError] = useState('')
  useEffect(() => { void get<Dashboard>('/api/v1/dashboard/executive').then(setData).catch(e => setError(e.message)) }, [])
  if (error) return <Box sx={pageSx}><Alert severity="error">{error}</Alert></Box>
  const utilization = data?.budgetTotal ? (data.actualCost / data.budgetTotal) * 100 : 0
  const metrics = [
    { label: 'Company Health', value: data ? `${data.kpiHealth.toFixed(1)}%` : '', icon: <FlagOutlined />, color: '#175cd3' },
    { label: 'Active Strategy', value: data?.strategyCount ?? '', icon: <AccountTreeOutlined />, color: '#087d70' },
    { label: 'Project Portfolio', value: data?.projectCount ?? '', icon: <BusinessCenterOutlined />, color: '#7b4ec7' },
    { label: 'Critical Risk', value: data?.highRiskProjects ?? '', icon: <CrisisAlertOutlined />, color: '#c53b3b' },
  ]
  return <Box sx={pageSx}>
    <PageHeader eyebrow="Home / Executive Cockpit" title={`안녕하세요, ${user?.displayName}님`} description="전사 전략과 실행 현황에서 오늘 확인해야 할 핵심 신호입니다." action={<Chip icon={<TrendingUpOutlined />} label="Live enterprise data" color="success" variant="outlined" />} />
    {user?.mustChangePassword && <Alert severity="warning" sx={{ mb: 3 }}>Bootstrap 관리자 비밀번호를 프로필 보안에서 변경해 주세요.</Alert>}
    <Grid container spacing={2.5}>{metrics.map(({ label, value, icon, color }) => <Grid key={label} size={{ xs: 12, sm: 6, xl: 3 }}><Card><CardContent sx={{ p: 2.5 }}><Stack direction="row" justifyContent="space-between"><Box><Typography color="text.secondary" variant="body2" fontWeight={650}>{label}</Typography>{data ? <Typography sx={{ fontSize: 30, fontWeight: 800, mt: .6 }}>{value}</Typography> : <Skeleton width={90} height={44} />}</Box><Box sx={{ width: 46, height: 46, borderRadius: 2, display: 'grid', placeItems: 'center', bgcolor: `${color}14`, color }}>{icon}</Box></Stack></CardContent></Card></Grid>)}</Grid>
    <Grid container spacing={2.5} sx={{ mt: .2 }}>
      <Grid size={{ xs: 12, lg: 7 }}><Card sx={{ height: '100%' }}><CardContent sx={{ p: 3 }}><Stack direction="row" justifyContent="space-between" mb={3}><Box><Typography variant="h3">Strategy Progress</Typography><Typography color="text.secondary" variant="body2">KPI 가중 성과 기준</Typography></Box><Typography fontWeight={800} color="primary.main">{data?.kpiHealth.toFixed(1) ?? 0}%</Typography></Stack><LinearProgress variant="determinate" value={Math.min(data?.kpiHealth ?? 0, 100)} sx={{ height: 12, borderRadius: 8, bgcolor: '#e8eef5' }} /><Grid container spacing={2} sx={{ mt: 2 }}>{[['목표 상태', (data?.kpiHealth ?? 0) >= 90 ? 'On track' : 'Attention'], ['검토 대기 계획', data?.pendingPlans ?? 0], ['누적 의사결정', data?.decisionCount ?? 0]].map(([label, value]) => <Grid key={String(label)} size={{ xs: 12, sm: 4 }}><Box sx={{ p: 2, bgcolor: '#f7f9fc', borderRadius: 2 }}><Typography variant="body2" color="text.secondary">{label}</Typography><Typography fontWeight={800} mt={.5}>{value}</Typography></Box></Grid>)}</Grid></CardContent></Card></Grid>
      <Grid size={{ xs: 12, lg: 5 }}><Card sx={{ height: '100%' }}><CardContent sx={{ p: 3 }}><Typography variant="h3">Financial Performance</Typography><Typography color="text.secondary" variant="body2" mb={3}>프로젝트 Portfolio 예산 집행</Typography><Stack direction="row" justifyContent="space-between"><Box><Typography variant="body2" color="text.secondary">Total Budget</Typography><Typography sx={{ fontSize: 24, fontWeight: 800 }}>{currency.format(data?.budgetTotal ?? 0)}</Typography></Box><Box textAlign="right"><Typography variant="body2" color="text.secondary">Actual Cost</Typography><Typography sx={{ fontSize: 24, fontWeight: 800 }}>{currency.format(data?.actualCost ?? 0)}</Typography></Box></Stack><LinearProgress color={utilization > 110 ? 'error' : 'secondary'} variant="determinate" value={Math.min(utilization, 100)} sx={{ mt: 3, height: 9, borderRadius: 7 }} /><Typography variant="body2" color="text.secondary" mt={1}>{utilization.toFixed(1)}% 집행</Typography></CardContent></Card></Grid>
      <Grid size={{ xs: 12 }}><Card><CardContent sx={{ p: 3 }}><Stack direction="row" spacing={1.5} alignItems="center" mb={2}><Box sx={{ color: '#b86e00' }}><LightbulbOutlined /></Box><Typography variant="h3">AI Executive Brief</Typography><Chip size="small" label="근거 기반" variant="outlined" /></Stack><Typography>{data ? `현재 KPI 종합 달성률은 ${data.kpiHealth.toFixed(1)}%이며, 즉시 확인이 필요한 고위험 프로젝트는 ${data.highRiskProjects}건입니다. 검토 또는 보완이 필요한 사업계획은 ${data.pendingPlans}건입니다.` : '경영 브리프를 구성하고 있습니다.'}</Typography><Typography variant="body2" color="text.secondary" mt={1.5}>Source: Planexus transactional data · deterministic brief</Typography></CardContent></Card></Grid>
    </Grid>
  </Box>
}
export const pageSx = { p: { xs: 2.5, md: 4, xl: 5 }, maxWidth: 1600, mx: 'auto' }
