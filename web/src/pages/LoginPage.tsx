import { useEffect, useState, type FormEvent } from 'react'
import { Alert, Box, Button, Card, CircularProgress, Divider, Stack, TextField, Typography } from '@mui/material'
import { ArrowForward, CorporateFareOutlined, InsightsOutlined, LockOutlined } from '@mui/icons-material'
import { useAuth } from '../auth/AuthContext'
import { get } from '../api/client'
import type { AuthConfig, Version } from '../types'

export function LoginPage() {
  const { login } = useAuth(); const [username, setUsername] = useState(''); const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false); const [error, setError] = useState(''); const [version, setVersion] = useState('…')
  const [config, setConfig] = useState<AuthConfig>({ localEnabled: true, oidcEnabled: false })
  useEffect(() => { void Promise.all([get<Version>('/api/v1/version'), get<AuthConfig>('/api/v1/auth/config')]).then(([v, c]) => { setVersion(v.version); setConfig(c) }).catch(() => {}) }, [])
  const submit = async (event: FormEvent) => { event.preventDefault(); setBusy(true); setError(''); try { await login(username, password) } catch (e) { setError(e instanceof Error ? e.message : '로그인하지 못했습니다.') } finally { setBusy(false) } }
  return <Box sx={{ minHeight: '100vh', display: 'grid', gridTemplateColumns: { xs: '1fr', lg: 'minmax(430px, 42%) 1fr' }, bgcolor: '#f4f7fb' }}>
    <Box sx={{ bgcolor: '#10243e', color: 'white', p: { xs: 4, md: 8 }, display: { xs: 'none', lg: 'flex' }, flexDirection: 'column', justifyContent: 'space-between', position: 'relative', overflow: 'hidden' }}>
      <Box sx={{ position: 'absolute', width: 520, height: 520, border: '1px solid #355579', borderRadius: '50%', right: -240, top: -150 }} /><Box sx={{ position: 'absolute', width: 360, height: 360, border: '1px solid #294969', borderRadius: '50%', right: -130, top: -70 }} />
      <Box sx={{ zIndex: 1 }}><Stack direction="row" spacing={1.5} alignItems="center"><Box sx={{ width: 44, height: 44, bgcolor: '#4d8eff', borderRadius: 2, display: 'grid', placeItems: 'center', fontWeight: 900, fontSize: 22 }}>P</Box><Typography sx={{ fontSize: 26, fontWeight: 850 }}>Planexus</Typography></Stack></Box>
      <Box sx={{ zIndex: 1, maxWidth: 550 }}><Typography sx={{ color: '#79a9ff', fontWeight: 750, mb: 2 }}>ENTERPRISE STRATEGY OPERATING SYSTEM</Typography><Typography component="h1" sx={{ fontSize: 44, lineHeight: 1.18, fontWeight: 800, letterSpacing: '-.035em', mb: 3 }}>전략을 실행으로,<br />데이터를 의사결정으로.</Typography><Typography sx={{ color: '#b8cbe1', fontSize: 18, lineHeight: 1.75 }}>전략·KPI·사업계획·프로젝트·예산·의사결정을 하나의 연결된 경영 사이클로 관리합니다.</Typography><Stack spacing={2.2} sx={{ mt: 5 }}>{[[<CorporateFareOutlined />, '전사 전략과 실행의 단일 정보원'], [<InsightsOutlined />, '근거 기반 성과 및 위험 인텔리전스'], [<LockOutlined />, '기업 권한과 보안등급에 맞춘 통제']].map(([icon, text]) => <Stack key={String(text)} direction="row" spacing={1.5} alignItems="center"><Box sx={{ color: '#79a9ff' }}>{icon}</Box><Typography sx={{ color: '#dce7f5' }}>{text}</Typography></Stack>)}</Stack></Box>
      <Typography sx={{ zIndex: 1, color: '#7890aa', fontSize: 13 }}>Air-gap ready · No external UI dependencies</Typography>
    </Box>
    <Box sx={{ p: { xs: 2.5, sm: 5 }, display: 'grid', placeItems: 'center' }}>
      <Card sx={{ width: '100%', maxWidth: 490, p: { xs: 3, sm: 5 } }}>
        <Box sx={{ display: { lg: 'none' }, mb: 4 }}><Typography sx={{ fontSize: 25, fontWeight: 850, color: '#10243e' }}>Planexus</Typography></Box>
        <Typography component="h1" variant="h2" mb={1}>다시 오신 것을 환영합니다</Typography><Typography color="text.secondary" mb={4}>회사 계정으로 Planexus에 로그인하세요.</Typography>
        {error && <Alert severity="error" sx={{ mb: 2 }}>{error}</Alert>}
        {config.oidcEnabled && <><Button fullWidth variant="contained" size="large" endIcon={<ArrowForward />} href="/api/v1/auth/oidc/login" sx={{ mb: 2 }}>{config.oidcLabel || 'Company SSO'}로 계속</Button><Divider sx={{ my: 2 }}>또는 로컬 관리자</Divider></>}
        <Box component="form" onSubmit={submit}><Stack spacing={2.2}><TextField label="사용자 이름" autoComplete="username" value={username} onChange={e => setUsername(e.target.value)} required fullWidth autoFocus={!config.oidcEnabled} /><TextField label="비밀번호" type="password" autoComplete="current-password" value={password} onChange={e => setPassword(e.target.value)} required fullWidth /><Button type="submit" variant={config.oidcEnabled ? 'outlined' : 'contained'} size="large" disabled={busy}>{busy ? <CircularProgress size={24} /> : '로그인'}</Button></Stack></Box>
        <Typography sx={{ mt: 4, color: 'text.secondary', fontSize: 13, textAlign: 'center' }}>Planexus v{version} · Strategy & Planning Intelligence Platform</Typography>
      </Card>
    </Box>
  </Box>
}
