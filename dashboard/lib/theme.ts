'use client';

// Google-product look: white surfaces, Material type scale, Google blue.

import { createTheme } from '@mui/material/styles';

export const GOOGLE_BLUE = '#1a73e8';
export const GOOGLE_BLUE_DARK = '#174ea6';
export const ERROR_RED = '#d93025';
export const WARNING_AMBER = '#f29900';
export const TEXT_SECONDARY = '#5f6368';
export const BORDER_GRAY = '#dadce0';

export const theme = createTheme({
  palette: {
    mode: 'light',
    primary: { main: GOOGLE_BLUE, dark: GOOGLE_BLUE_DARK },
    error: { main: ERROR_RED },
    warning: { main: WARNING_AMBER },
    text: { primary: '#202124', secondary: TEXT_SECONDARY },
    divider: BORDER_GRAY,
    background: { default: '#f8f9fa', paper: '#ffffff' },
  },
  typography: {
    fontFamily:
      'Roboto, "Helvetica Neue", Arial, sans-serif',
    h6: { fontWeight: 500 },
    subtitle2: { color: TEXT_SECONDARY },
    body2: { fontSize: 13 },
    caption: { color: TEXT_SECONDARY },
  },
  shape: { borderRadius: 8 },
  components: {
    MuiPaper: {
      styleOverrides: {
        root: {
          boxShadow: 'none',
          border: `1px solid ${BORDER_GRAY}`,
        },
      },
    },
    MuiAppBar: {
      styleOverrides: {
        root: {
          backgroundColor: '#ffffff',
          color: '#202124',
          boxShadow: 'none',
          borderBottom: `1px solid ${BORDER_GRAY}`,
        },
      },
    },
    MuiTableCell: {
      styleOverrides: {
        root: { borderBottomColor: '#f1f3f4', padding: '6px 12px' },
        head: {
          color: TEXT_SECONDARY,
          fontWeight: 500,
          fontSize: 12,
          backgroundColor: '#ffffff',
        },
      },
    },
    MuiChip: {
      styleOverrides: { root: { fontWeight: 500 } },
    },
  },
});
