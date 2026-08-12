import { render, screen } from '@testing-library/react'
import { ThemeProvider } from '@mui/material'
import { expect, test } from 'vitest'
import { StatusChip } from '../components/StatusChip'
import { theme } from '../theme'

test('renders readable workflow status', () => {
  render(<ThemeProvider theme={theme}><StatusChip value="in_review" /></ThemeProvider>)
  expect(screen.getByText('in review')).toBeInTheDocument()
})
