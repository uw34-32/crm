// A searchable, paged list of one kind of record, with its empty, loading, and error states.

import { ActivityIndicator, FlatList, Pressable, StyleSheet, Text, View } from 'react-native';
import { router, type Href } from 'expo-router';
import { SafeAreaView } from 'react-native-safe-area-context';
import type { Page } from '../api/records';
import { useRecordList } from '../lib/records/hooks';
import type { FetchPage } from '../lib/records/list';
import { radius, useTheme } from '../theme';
import { Button } from './Button';
import { ErrorNotice } from './ErrorNotice';
import { RecordAvatar } from './RecordAvatar';
import { SearchField } from './SearchField';

export type Row = { name: string; subtitle: string; href: Href };

type Props<T> = {
  title: string;
  /** The plural noun, in lowercase: "contacts". */
  noun: string;
  newLabel: string;
  newHref: Href;
  fetchPage: (workspaceId: string, params: Parameters<FetchPage<T>>[0]) => Promise<Page<T>>;
  row: (item: T) => Row;
};

export function RecordList<T extends { id: string }>({ title, noun, newLabel, newHref, fetchPage, row }: Props<T>) {
  const theme = useTheme();
  const { state, search, setSearch, more, retry } = useRecordList<T>(fetchPage);
  const searching = state.q !== '';

  const header = (
    <View style={styles.header}>
      <Text accessibilityRole="header" style={[styles.title, { color: theme.text }]}>
        {title}
      </Text>
      <SearchField value={search} onChangeText={setSearch} placeholder={`Search ${noun}`} />
      <Button label={newLabel} onPress={() => router.push(newHref)} />
    </View>
  );

  let empty = null;
  if (state.status === 'error') empty = <ErrorNotice message={state.error} onRetry={retry} />;
  else if (state.status !== 'ready') empty = <ActivityIndicator accessibilityLabel={`Loading ${noun}`} style={styles.spinner} />;
  else if (state.items.length === 0)
    empty = (
      <Text style={{ color: theme.text2, textAlign: 'center', padding: 24 }}>
        {searching ? `No ${noun} match “${state.q}”.` : `No ${noun} yet. Add the first one.`}
      </Text>
    );

  const footer = state.moreError ? (
    <ErrorNotice message={state.moreError} onRetry={more} />
  ) : state.loadingMore ? (
    <ActivityIndicator accessibilityLabel={`Loading more ${noun}`} style={styles.spinner} />
  ) : null;

  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: theme.bg }} edges={['top', 'left', 'right']}>
      <FlatList
        data={state.status === 'ready' ? state.items : []}
        keyExtractor={(item) => item.id}
        contentContainerStyle={styles.content}
        ListHeaderComponent={header}
        ListEmptyComponent={empty ?? undefined}
        ListFooterComponent={footer ?? undefined}
        keyboardShouldPersistTaps="handled"
        onEndReached={() => void more()}
        onEndReachedThreshold={0.5}
        renderItem={({ item }) => {
          const { name, subtitle, href } = row(item);
          return (
            <Pressable
              accessibilityRole="link"
              accessibilityLabel={subtitle ? `${name}, ${subtitle}` : name}
              onPress={() => router.push(href)}
              style={({ pressed }) => [styles.row, { backgroundColor: theme.surface, borderColor: theme.border, opacity: pressed ? 0.85 : 1 }]}
            >
              <RecordAvatar name={name} />
              <View style={{ flex: 1 }}>
                <Text numberOfLines={1} style={[styles.name, { color: theme.text }]}>
                  {name}
                </Text>
                {subtitle ? (
                  <Text numberOfLines={1} style={{ color: theme.text2 }}>
                    {subtitle}
                  </Text>
                ) : null}
              </View>
            </Pressable>
          );
        }}
      />
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  content: { padding: 16, gap: 10, maxWidth: 640, width: '100%', alignSelf: 'center' },
  header: { gap: 12, marginBottom: 6 },
  title: { fontSize: 28, fontWeight: '700' },
  spinner: { marginVertical: 24 },
  row: { flexDirection: 'row', alignItems: 'center', gap: 12, borderWidth: 1, borderRadius: radius, padding: 12, minHeight: 64 },
  name: { fontSize: 16, fontWeight: '600' }
});
