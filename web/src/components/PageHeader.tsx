import { Box, Breadcrumbs, Typography } from '@mui/material'
import type { ReactNode } from 'react'

export function PageHeader({ eyebrow, title, description, action }: { eyebrow?: string; title: string; description?: string; action?: ReactNode }) {
  return <Box sx={{ display: 'flex', gap: 3, alignItems: 'flex-start', justifyContent: 'space-between', mb: 3.5 }}><Box>{eyebrow && <Breadcrumbs sx={{ mb: 1 }}><Typography color="text.secondary" variant="body2">{eyebrow}</Typography></Breadcrumbs>}<Typography component="h1" variant="h1">{title}</Typography>{description && <Typography color="text.secondary" sx={{ mt: 1, maxWidth: 760 }}>{description}</Typography>}</Box>{action}</Box>
}
