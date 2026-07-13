'use client';

import Paper from '@mui/material/Paper';
import Typography from '@mui/material/Typography';
import ToggleButton from '@mui/material/ToggleButton';
import ToggleButtonGroup from '@mui/material/ToggleButtonGroup';
import FormGroup from '@mui/material/FormGroup';
import FormControlLabel from '@mui/material/FormControlLabel';
import Checkbox from '@mui/material/Checkbox';
import Divider from '@mui/material/Divider';
import { PRESETS } from '@/lib/buckets';

export interface TraceFilters {
  withErrors: boolean;
  withoutErrors: boolean;
  withWarnings: boolean;
  withoutWarnings: boolean;
}

export const DEFAULT_FILTERS: TraceFilters = {
  withErrors: true,
  withoutErrors: true,
  withWarnings: true,
  withoutWarnings: true,
};

function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <Typography
      variant="caption"
      sx={{
        display: 'block',
        textTransform: 'uppercase',
        letterSpacing: 0.5,
        fontSize: 11,
        mb: 0.5,
      }}
    >
      {children}
    </Typography>
  );
}

export default function FiltersPanel({
  presetKey,
  onPresetChange,
  filters,
  onFiltersChange,
}: {
  presetKey: string;
  onPresetChange: (key: string) => void;
  filters: TraceFilters;
  onFiltersChange: (f: TraceFilters) => void;
}) {
  const check = (key: keyof TraceFilters) => (
    <Checkbox
      size="small"
      checked={filters[key]}
      onChange={(e) => onFiltersChange({ ...filters, [key]: e.target.checked })}
      sx={{ py: 0.5 }}
    />
  );

  return (
    <Paper sx={{ width: 220, flexShrink: 0, p: 2, alignSelf: 'flex-start' }}>
      <Typography variant="subtitle2" sx={{ mb: 1.5, color: 'text.primary' }}>
        Filters
      </Typography>

      <SectionLabel>Focus time</SectionLabel>
      <ToggleButtonGroup
        exclusive
        orientation="vertical"
        fullWidth
        size="small"
        value={presetKey}
        onChange={(_, v) => v && onPresetChange(v)}
        sx={{
          mb: 2,
          '& .MuiToggleButton-root': {
            justifyContent: 'flex-start',
            textTransform: 'none',
            py: 0.4,
            fontSize: 13,
          },
        }}
      >
        {PRESETS.map((p) => (
          <ToggleButton key={p.key} value={p.key}>
            {p.label}
          </ToggleButton>
        ))}
      </ToggleButtonGroup>

      <Divider sx={{ mb: 1.5 }} />

      <SectionLabel>Errors</SectionLabel>
      <FormGroup sx={{ mb: 1.5 }}>
        <FormControlLabel
          control={check('withErrors')}
          label="With errors"
          slotProps={{ typography: { variant: 'body2' } }}
        />
        <FormControlLabel
          control={check('withoutErrors')}
          label="Without errors"
          slotProps={{ typography: { variant: 'body2' } }}
        />
      </FormGroup>

      <SectionLabel>Flags</SectionLabel>
      <FormGroup>
        <FormControlLabel
          control={check('withWarnings')}
          label="With warnings"
          slotProps={{ typography: { variant: 'body2' } }}
        />
        <FormControlLabel
          control={check('withoutWarnings')}
          label="Without warnings"
          slotProps={{ typography: { variant: 'body2' } }}
        />
      </FormGroup>
    </Paper>
  );
}
