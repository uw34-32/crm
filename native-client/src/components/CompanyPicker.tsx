// Picks the company a contact is linked to by searching the workspace's companies. The
// text is only for searching: a company is linked by choosing it from the list, and
// unlinked with Remove.

import { useEffect, useRef, useState } from 'react';
import { ActivityIndicator, Pressable, StyleSheet, Text, View } from 'react-native';
import { recordsApi, type Company } from '../api/records';
import { debounce } from '../lib/debounce';
import { session } from '../services';
import { radius, useTheme } from '../theme';
import { Button } from './Button';
import { Field } from './Field';

export type Choice = { id: string; name: string };

const RESULTS = 8;
const companies = recordsApi(session.api, 'companies');

type Props = { workspaceId: string; value: Choice | null; onChange: (value: Choice | null) => void; error?: string };

export function CompanyPicker({ workspaceId, value, onChange, error = '' }: Props) {
  const theme = useTheme();
  const [query, setQuery] = useState('');
  const [options, setOptions] = useState<Company[]>([]);
  const [status, setStatus] = useState<'idle' | 'loading' | 'ready' | 'error'>('idle');
  const generation = useRef(0);
  // The debounced search outlives renders, so it reads the workspace from here.
  const workspace = useRef(workspaceId);
  workspace.current = workspaceId;

  const search = useRef(
    debounce(async (text: string) => {
      const mine = ++generation.current;
      setStatus('loading');
      try {
        const page = await companies.list(workspace.current, { q: text.trim(), limit: RESULTS, offset: 0 });
        if (mine !== generation.current) return;
        setOptions(page.items);
        setStatus('ready');
      } catch {
        if (mine !== generation.current) return;
        setOptions([]);
        setStatus('error');
      }
    }, 250)
  ).current;

  useEffect(
    () => () => {
      search.cancel();
      generation.current++;
    },
    [search]
  );

  if (value) {
    return (
      <View style={styles.group}>
        <Text style={[styles.label, { color: theme.text }]}>Company</Text>
        <View style={[styles.chosen, { backgroundColor: theme.surface2, borderColor: theme.border }]}>
          <Text style={{ color: theme.text, fontSize: 16, flex: 1 }}>{value.name}</Text>
          <Pressable
            accessibilityRole="button"
            accessibilityLabel={`Remove ${value.name}`}
            hitSlop={8}
            onPress={() => {
              onChange(null);
              setQuery('');
              setStatus('idle');
            }}
          >
            <Text style={{ color: theme.dangerText, fontWeight: '600' }}>Remove</Text>
          </Pressable>
        </View>
        {error ? <Text style={{ color: theme.dangerText, fontSize: 13 }}>{error}</Text> : null}
      </View>
    );
  }

  return (
    <View style={styles.group}>
      <Field
        label="Company"
        value={query}
        error={error}
        placeholder="Search companies"
        autoCapitalize="none"
        autoCorrect={false}
        onFocus={() => status === 'idle' && void search(query)}
        onChangeText={(text) => {
          setQuery(text);
          void search(text);
        }}
      />
      {status === 'loading' ? <ActivityIndicator accessibilityLabel="Searching companies" /> : null}
      {status === 'error' ? <Text style={{ color: theme.dangerText }}>The companies could not be loaded. Keep typing to try again.</Text> : null}
      {status === 'ready' && options.length === 0 ? <Text style={{ color: theme.text2 }}>No companies match. Add one first, then link it here.</Text> : null}
      {status === 'ready'
        ? options.map((company) => (
            <Pressable
              key={company.id}
              accessibilityRole="button"
              onPress={() => {
                onChange({ id: company.id, name: company.name });
                setOptions([]);
              }}
              style={[styles.option, { backgroundColor: theme.surface, borderColor: theme.border }]}
            >
              <Text style={{ color: theme.text, fontSize: 16 }}>{company.name}</Text>
              {company.domain ? <Text style={{ color: theme.text2 }}>{company.domain}</Text> : null}
            </Pressable>
          ))
        : null}
      {status === 'ready' && options.length === 0 && query ? <Button label="Clear search" variant="secondary" onPress={() => { setQuery(''); void search(''); }} /> : null}
    </View>
  );
}

const styles = StyleSheet.create({
  group: { gap: 8 },
  label: { fontSize: 14, fontWeight: '600' },
  chosen: { flexDirection: 'row', alignItems: 'center', gap: 12, minHeight: 48, borderWidth: 1, borderRadius: radius, paddingHorizontal: 14 },
  option: { borderWidth: 1, borderRadius: radius, padding: 12, minHeight: 48, justifyContent: 'center' }
});
