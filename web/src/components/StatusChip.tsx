import { Chip } from '@mui/material'

export function StatusChip({ value }: { value: string }) {
  const color = ['high', 'critical', 'rejected'].includes(value) ? 'error' : ['confirmed', 'active', 'approved', 'low'].includes(value) ? 'success' : ['in_review', 'medium', 'draft'].includes(value) ? 'warning' : 'default'
  return <Chip label={value.replaceAll('_', ' ')} color={color} variant="outlined" size="small" sx={{ fontWeight: 700, textTransform: 'capitalize' }} />
}
