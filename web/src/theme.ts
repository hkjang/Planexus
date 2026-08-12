import { createTheme } from '@mui/material/styles'

export const theme = createTheme({
  palette: {
    mode: 'light',
    primary: { main: '#175cd3', dark: '#10243e', light: '#e8f1ff' },
    secondary: { main: '#087d70' },
    background: { default: '#f5f7fa', paper: '#ffffff' },
    text: { primary: '#182230', secondary: '#536071' },
    error: { main: '#c53b3b' },
    warning: { main: '#b86e00' },
    success: { main: '#16805d' },
  },
  typography: {
    fontFamily: 'Pretendard, "Noto Sans KR", "Segoe UI", Arial, sans-serif',
    fontSize: 16,
    htmlFontSize: 16,
    h1: { fontSize: '2rem', fontWeight: 750, letterSpacing: '-0.025em' },
    h2: { fontSize: '1.55rem', fontWeight: 750, letterSpacing: '-0.02em' },
    h3: { fontSize: '1.2rem', fontWeight: 700 },
    body1: { fontSize: '1rem', lineHeight: 1.65 },
    body2: { fontSize: '0.925rem', lineHeight: 1.55 },
    button: { fontSize: '0.925rem', fontWeight: 700, textTransform: 'none' },
  },
  shape: { borderRadius: 10 },
  components: {
    MuiButton: { defaultProps: { disableElevation: true }, styleOverrides: { root: { minHeight: 42, paddingInline: 18 } } },
    MuiTextField: { defaultProps: { size: 'small' } },
    MuiTableCell: { styleOverrides: { root: { fontSize: '0.925rem', borderColor: '#e5e9ef' }, head: { fontWeight: 750, color: '#3d4b5f', backgroundColor: '#f8fafc' } } },
    MuiCard: { styleOverrides: { root: { border: '1px solid #e5e9ef', boxShadow: '0 1px 3px rgba(16,36,62,.05)' } } },
    MuiTooltip: { styleOverrides: { tooltip: { fontSize: '0.85rem' } } },
  },
})
