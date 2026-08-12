import { useEffect, useMemo, useState } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import {
  AppBar, Autocomplete, Avatar, Badge, Box, CircularProgress, Divider, Drawer, IconButton, InputAdornment, List, ListItemButton,
  ListItemIcon, ListItemText, Menu, MenuItem, TextField, Toolbar, Tooltip, Typography, useMediaQuery,
} from '@mui/material'
import {
  AccountTreeOutlined, AdminPanelSettingsOutlined, AssessmentOutlined, BusinessCenterOutlined,
  DashboardOutlined, GavelOutlined, KeyOutlined, LogoutOutlined, MenuOutlined, PersonOutline,
  NotificationsOutlined, PsychologyOutlined, RadarOutlined, ScienceOutlined, SearchOutlined, SpaceDashboardOutlined, TrackChangesOutlined,
} from '@mui/icons-material'
import { useTheme } from '@mui/material/styles'
import { useAuth } from '../auth/AuthContext'
import { get } from '../api/client'
import { post } from '../api/client'
import type { Version } from '../types'

const drawerWidth = 272
type SearchResult = { id: string; type: string; title: string; subtitle: string; classification: string }
const searchRoutes: Record<string, string> = { strategy: '/strategy', kpi: '/kpi', project: '/projects', plan: '/plans', decision: '/decisions', intelligence: '/intelligence' }
const primary = [
  { label: 'Executive Cockpit', path: '/', icon: <DashboardOutlined />, permission: 'dashboard:read' },
  { label: 'Strategy', path: '/strategy', icon: <AccountTreeOutlined />, permission: 'strategy:read' },
  { label: 'KPI & Performance', path: '/kpi', icon: <TrackChangesOutlined />, permission: 'kpi:read' },
  { label: 'Project Portfolio', path: '/projects', icon: <BusinessCenterOutlined />, permission: 'project:read' },
  { label: 'Business Planning', path: '/plans', icon: <AssessmentOutlined />, permission: 'plan:own' },
  { label: 'Decision Log', path: '/decisions', icon: <GavelOutlined />, permission: 'decision:read' },
  { label: 'Intelligence Hub', path: '/intelligence', icon: <RadarOutlined />, permission: 'intelligence:read' },
  { label: 'Scenario Planning', path: '/scenarios', icon: <ScienceOutlined />, permission: 'scenario:read' },
  { label: 'AI Planning Assistant', path: '/ai', icon: <PsychologyOutlined />, permission: 'ai:query' },
]

export function AppShell() {
  const { user, logout, has } = useAuth()
  const navigate = useNavigate(); const location = useLocation(); const theme = useTheme()
  const desktop = useMediaQuery(theme.breakpoints.up('lg'))
  const [mobileOpen, setMobileOpen] = useState(false)
  const [profileAnchor, setProfileAnchor] = useState<HTMLElement | null>(null)
  const [notificationAnchor,setNotificationAnchor]=useState<HTMLElement|null>(null)
  const [notifications,setNotifications]=useState<Array<{id:string;severity:string;title:string;message:string;createdAt:string}>>([])
  const [searchText,setSearchText]=useState(''); const [searchResults,setSearchResults]=useState<SearchResult[]>([]); const [searching,setSearching]=useState(false)
  const [version, setVersion] = useState<Version>({ service: 'Planexus', version: '…' })
  useEffect(() => { void get<Version>('/api/v1/version').then(setVersion);void get<typeof notifications>('/api/v1/notifications?unread=true').then(setNotifications).catch(()=>{}) }, [])
  useEffect(()=>{if(searchText.trim().length<2){setSearchResults([]);setSearching(false);return}setSearching(true);const timer=window.setTimeout(()=>{void get<SearchResult[]>(`/api/v1/search?q=${encodeURIComponent(searchText.trim())}`).then(setSearchResults).catch(()=>setSearchResults([])).finally(()=>setSearching(false))},250);return()=>window.clearTimeout(timer)},[searchText])
  const canAdmin = useMemo(() => has('*'), [has])
  const go = (path: string) => { navigate(path); setMobileOpen(false) }
  const drawer = <Box sx={{ height: '100%', bgcolor: '#10243e', color: '#dce7f5', display: 'flex', flexDirection: 'column' }}>
    <Box sx={{ px: 3, py: 2.5, display: 'flex', gap: 1.3, alignItems: 'center' }}>
      <Box sx={{ width: 34, height: 34, borderRadius: 2, bgcolor: '#4d8eff', display: 'grid', placeItems: 'center', fontWeight: 900, color: 'white' }}>P</Box>
      <Box><Typography sx={{ fontSize: 20, fontWeight: 800, color: 'white', lineHeight: 1.1 }}>Planexus</Typography><Typography sx={{ fontSize: 11, color: '#91a8c4', letterSpacing: '.08em' }}>STRATEGY OS</Typography></Box>
    </Box>
    <Divider sx={{ borderColor: '#29425f' }} />
    <Box className="admin-scroll" sx={{ overflowY: 'auto', px: 1.5, py: 2, flex: 1 }}>
      <Typography sx={{ px: 1.5, pb: 1, color: '#91a8c4', fontWeight: 700, fontSize: 12, letterSpacing: '.08em' }}>ENTERPRISE</Typography>
      <List disablePadding>{primary.filter(item => has(item.permission)).map(item => <ListItemButton key={item.path} selected={location.pathname === item.path} onClick={() => go(item.path)} sx={navSx}><ListItemIcon sx={{ minWidth: 40, color: 'inherit' }}>{item.icon}</ListItemIcon><ListItemText primary={item.label} primaryTypographyProps={{ fontSize: 14.5, fontWeight: 650 }} /></ListItemButton>)}</List>
      <Typography sx={{ px: 1.5, pt: 3, pb: 1, color: '#91a8c4', fontWeight: 700, fontSize: 12, letterSpacing: '.08em' }}>PERSONAL</Typography>
      <List disablePadding><ListItemButton selected={location.pathname === '/personal'} onClick={() => go('/personal')} sx={navSx}><ListItemIcon sx={{ minWidth: 40, color: 'inherit' }}><SpaceDashboardOutlined /></ListItemIcon><ListItemText primary="My Workspace" primaryTypographyProps={{ fontSize: 14.5, fontWeight: 650 }} /></ListItemButton></List>
      {canAdmin && <><Typography sx={{ px: 1.5, pt: 3, pb: 1, color: '#91a8c4', fontWeight: 700, fontSize: 12, letterSpacing: '.08em' }}>SERVICE ADMINISTRATION</Typography><List disablePadding><ListItemButton selected={location.pathname.startsWith('/admin')} onClick={() => go('/admin/system')} sx={navSx}><ListItemIcon sx={{ minWidth: 40, color: 'inherit' }}><AdminPanelSettingsOutlined /></ListItemIcon><ListItemText primary="Administration" primaryTypographyProps={{ fontSize: 14.5, fontWeight: 650 }} /></ListItemButton></List></>}
    </Box>
    <Box sx={{ p: 2, borderTop: '1px solid #29425f' }}><Typography sx={{ fontSize: 12, color: '#91a8c4' }}>Enterprise Strategy Intelligence</Typography></Box>
  </Box>

  return <Box sx={{ display: 'flex', minHeight: '100vh' }}>
    <AppBar position="fixed" color="inherit" elevation={0} sx={{ zIndex: t => t.zIndex.drawer - (desktop ? 0 : 1), ml: desktop ? `${drawerWidth}px` : 0, width: desktop ? `calc(100% - ${drawerWidth}px)` : '100%', borderBottom: '1px solid #e3e8ef' }}>
      <Toolbar sx={{ minHeight: '72px!important', gap: 2 }}>
        {!desktop && <IconButton aria-label="메뉴 열기" onClick={() => setMobileOpen(true)}><MenuOutlined /></IconButton>}
		<Autocomplete<SearchResult,false,false,false>
		  options={searchResults} loading={searching} inputValue={searchText}
		  onInputChange={(_,value)=>setSearchText(value)} filterOptions={options=>options}
		  getOptionLabel={option=>option.title} isOptionEqualToValue={(a,b)=>a.id===b.id}
		  onChange={(_,item)=>{if(item){navigate(searchRoutes[item.type]??'/');setSearchText('');setSearchResults([])}}}
		  noOptionsText={searchText.trim().length<2?'두 글자 이상 입력하세요':'권한 범위 내 검색 결과가 없습니다.'}
		  sx={{maxWidth:620,flex:1}}
		  renderOption={(props,option)=><li {...props} key={`${option.type}-${option.id}`}><Box><Typography fontWeight={750}>{option.title}</Typography><Typography variant="body2" color="text.secondary">{option.type.toUpperCase()} · {option.subtitle||option.classification}</Typography></Box></li>}
		  renderInput={params=><TextField {...params} placeholder="전략, KPI, 프로젝트, 의사결정 통합 검색" aria-label="통합 검색" InputProps={{...params.InputProps,startAdornment:<><InputAdornment position="start"><SearchOutlined /></InputAdornment>{params.InputProps.startAdornment}</>,endAdornment:<>{searching?<CircularProgress size={18}/>:null}{params.InputProps.endAdornment}</>,sx:{bgcolor:'#f4f6f9'}}}/>}
		/>
        <Box sx={{ flex: 1 }} />
        <Tooltip title="알림"><IconButton aria-label={`읽지 않은 알림 ${notifications.length}건`} onClick={e=>setNotificationAnchor(e.currentTarget)}><Badge badgeContent={notifications.length} color="error"><NotificationsOutlined/></Badge></IconButton></Tooltip>
        <Menu anchorEl={notificationAnchor} open={!!notificationAnchor} onClose={()=>setNotificationAnchor(null)} slotProps={{paper:{sx:{width:360,maxHeight:480}}}}><Box sx={{px:2,py:1.5}}><Typography fontWeight={800}>알림</Typography><Typography variant="body2" color="text.secondary">읽지 않은 알림 {notifications.length}건</Typography></Box><Divider/>{notifications.length===0?<Box sx={{p:3,textAlign:'center'}}><Typography color="text.secondary">새 알림이 없습니다.</Typography></Box>:notifications.map(item=><MenuItem key={item.id} onClick={()=>{void post(`/api/v1/notifications/${item.id}/read`).then(()=>setNotifications(v=>v.filter(x=>x.id!==item.id)))}} sx={{whiteSpace:'normal',alignItems:'flex-start',py:1.5}}><Box><Typography fontWeight={700} color={item.severity==='critical'?'error.main':'text.primary'}>{item.title}</Typography><Typography variant="body2" color="text.secondary" sx={{mt:.4}}>{item.message}</Typography><Typography sx={{fontSize:11,color:'text.disabled',mt:.5}}>{new Date(item.createdAt).toLocaleString('ko-KR')}</Typography></Box></MenuItem>)}</Menu>
        <Tooltip title="프로필 메뉴"><IconButton onClick={e => setProfileAnchor(e.currentTarget)} aria-label="프로필 메뉴" sx={{ borderRadius: 2, gap: 1 }}><Avatar sx={{ width: 36, height: 36, bgcolor: '#dce9ff', color: '#175cd3', fontSize: 15, fontWeight: 800 }}>{user?.displayName?.slice(0, 1)}</Avatar><Box sx={{ display: { xs: 'none', md: 'block' }, textAlign: 'left' }}><Typography sx={{ fontSize: 14, fontWeight: 750 }}>{user?.displayName}</Typography><Typography sx={{ fontSize: 12, color: 'text.secondary' }}>{user?.title || user?.roles[0]}</Typography></Box></IconButton></Tooltip>
        <Menu anchorEl={profileAnchor} open={!!profileAnchor} onClose={() => setProfileAnchor(null)} slotProps={{ paper: { sx: { width: 280, mt: 1 } } }}>
          <Box sx={{ px: 2, py: 1.5 }}><Typography fontWeight={750}>{user?.displayName}</Typography><Typography variant="body2" color="text.secondary">{user?.email || user?.username}</Typography></Box><Divider />
          <MenuItem onClick={() => { navigate('/personal'); setProfileAnchor(null) }}><PersonOutline sx={{ mr: 1.5 }} />개인화 페이지</MenuItem>
          <MenuItem onClick={() => { navigate('/profile/security'); setProfileAnchor(null) }}><KeyOutlined sx={{ mr: 1.5 }} />비밀번호 및 API 키</MenuItem>
          <Divider /><Box sx={{ px: 2, py: 1.2 }}><Typography sx={{ fontSize: 12, color: 'text.secondary' }}>Planexus v{version.version}</Typography></Box><Divider />
          <MenuItem onClick={() => void logout()}><LogoutOutlined sx={{ mr: 1.5 }} />로그아웃</MenuItem>
        </Menu>
      </Toolbar>
    </AppBar>
    <Box component="nav" sx={{ width: { lg: drawerWidth }, flexShrink: { lg: 0 } }}>
      <Drawer variant={desktop ? 'permanent' : 'temporary'} open={desktop || mobileOpen} onClose={() => setMobileOpen(false)} ModalProps={{ keepMounted: true }} sx={{ '& .MuiDrawer-paper': { width: drawerWidth, border: 0 } }}>{drawer}</Drawer>
    </Box>
    <Box component="main" sx={{ flex: 1, minWidth: 0, pt: '72px' }}><Outlet /></Box>
  </Box>
}

const navSx = { color: '#c8d6e7', borderRadius: 1.5, mb: .5, minHeight: 46, '&:hover': { bgcolor: '#1a3658', color: 'white' }, '&.Mui-selected': { bgcolor: '#234a78', color: 'white', '&:hover': { bgcolor: '#2a568a' }, '&:before': { content: '""', position: 'absolute', left: 0, width: 3, height: 24, bgcolor: '#70a5ff', borderRadius: 3 } } }
