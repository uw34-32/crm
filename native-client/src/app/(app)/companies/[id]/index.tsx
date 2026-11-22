import { useEffect, useState } from 'react';
import { ActivityIndicator, Pressable, StyleSheet, Text, View } from 'react-native';
import { router, useLocalSearchParams } from 'expo-router';
import { recordsApi, type Contact } from '../../../../api/records';
import { Button } from '../../../../components/Button';
import { ActivityTimeline } from '../../../../components/ActivityTimeline';
import { ConfirmDialog } from '../../../../components/ConfirmDialog';
import { DetailCard, DetailRow } from '../../../../components/DetailCard';
import { ErrorNotice } from '../../../../components/ErrorNotice';
import { NotFound } from '../../../../components/NotFound';
import { RecordAvatar } from '../../../../components/RecordAvatar';
import { ScreenFrame } from '../../../../components/ScreenFrame';
import { TextAction } from '../../../../components/TextAction';
import { toSentence } from '../../../../lib/forms';
import { useStore } from '../../../../lib/store';
import { changes, recordsChanged } from '../../../../lib/records/changes';
import { contactName, contactSubtitle } from '../../../../lib/records/format';
import { useLeaveOnWorkspaceSwitch, useRecord } from '../../../../lib/records/hooks';
import { createListStore, createPager } from '../../../../lib/records/list';
import { isMissing } from '../../../../lib/records/load';
import { isPersonOf, loadPeople } from '../../../../lib/records/people';
import { session } from '../../../../services';
import { radius, useTheme } from '../../../../theme';
import { dateLocale, t, useLocale } from '../../../../i18n';

const companies = recordsApi(session.api, 'companies');
const contactsApi = recordsApi(session.api, 'contacts');
const formatDate = (value: Date) => new Intl.DateTimeFormat(dateLocale(), { dateStyle: 'medium' }).format(value);

export default function CompanyDetail() {
  useLocale();
  const theme = useTheme();
  const { id } = useLocalSearchParams<{ id: string }>();
  useLeaveOnWorkspaceSwitch('/companies');
  const [gone, setGone] = useState(false);
  // Once deleted, the record is not loaded again: it would answer 404 while the screen closes.
  const { state, workspaceId, reload } = useRecord(companies.get, id, { paused: gone });
  const { version } = useStore(changes);
  const company = state.record;

  const [{ store, pager }] = useState(() => {
    const peopleStore = createListStore<Contact>();
    return { store: peopleStore, pager: { current: null as ReturnType<typeof createPager<Contact>> | null } };
  });
  const people = useStore(store);

  // The pager filters on this company's ID, so it is built once the company is known. It
  // depends on the id and name, not the record, which is replaced on every refresh.
  const companyId = company?.id;
  const companyName = company?.name;
  useEffect(() => {
    if (!companyId || companyName === undefined || !workspaceId || gone) return;
    const mine = createPager(store, (params) => contactsApi.list(workspaceId, params), { keep: isPersonOf(companyId) });
    pager.current = mine;
    let current = true;
    void loadPeople(mine, store, companyName, () => current);
    return () => {
      current = false;
      mine.stop();
    };
  }, [companyId, companyName, workspaceId, store, pager, version, gone]);

  const [confirming, setConfirming] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState('');

  if (state.status === 'missing') return <NotFound title={t('Company not found')} backHref="/companies" backLabel={t('companies')} />;

  async function remove() {
    if (!company || !workspaceId) return;
    setDeleting(true);
    setDeleteError('');
    try {
      await companies.remove(workspaceId, company.id);
    } catch (err) {
      if (!isMissing(err)) {
        setDeleteError(toSentence(err instanceof Error ? err.message : String(err)));
        setDeleting(false);
        return;
      }
    }
    setGone(true);
    setConfirming(false);
    setDeleting(false);
    router.dismissTo('/companies');
    recordsChanged();
  }

  const details = company ? ([[t('Domain'), company.domain], [t('Phone'), company.phone], [t('Industry'), company.industry], [t('Address'), company.address]] as const).filter(([, value]) => value) : [];

  return (
    <ScreenFrame
      backLabel={t('Companies')}
      backHref="/companies"
      actions={
        company ? (
          <>
            <TextAction label={t('Edit')} onPress={() => router.push(`/companies/${company.id}/edit`)} />
            <TextAction label={t('Delete')} danger onPress={() => setConfirming(true)} />
          </>
        ) : null
      }
    >
      {state.status === 'error' ? (
        <>
          <Text accessibilityRole="header" style={[styles.title, { color: theme.text }]}>
            {t('Company')}
          </Text>
          <ErrorNotice message={state.error} onRetry={() => void reload()} />
        </>
      ) : !company ? (
        <ActivityIndicator accessibilityLabel={t('Loading the company')} />
      ) : (
        <>
          <View style={styles.hero}>
            <RecordAvatar name={company.name} size={64} />
            <View style={{ flex: 1 }}>
              <Text accessibilityRole="header" style={[styles.title, { color: theme.text }]}>
                {company.name}
              </Text>
              {company.industry ? <Text style={{ color: theme.text2, fontSize: 16 }}>{company.industry}</Text> : null}
            </View>
          </View>
          <DetailCard title={t('Details')}>
            {details.length > 0 ? (
              details.map(([label, value]) => (
                <DetailRow
                  key={label}
                  label={label}
                  value={value as string}
                  url={label === t('Phone') ? `tel:${value}` : label === t('Domain') ? `https://${value}` : undefined}
                />
              ))
            ) : (
              <Text style={{ color: theme.text2 }}>{t('No domain, phone, industry, or address yet.')}</Text>
            )}
          </DetailCard>
          <DetailCard title={t('People')}>
            {people.status === 'error' ? (
              <ErrorNotice message={people.error} onRetry={() => company && pager.current && void loadPeople(pager.current, store, company.name)} />
            ) : people.status !== 'ready' ? (
              <ActivityIndicator accessibilityLabel={t('Loading people')} />
            ) : people.items.length === 0 ? (
              <Text style={{ color: theme.text2 }}>{people.hasMore ? t('None found in the first results. Look further to check more.') : t('No one is linked to this company yet.')}</Text>
            ) : (
              people.items.map((person) => (
                <Pressable
                  key={person.id}
                  accessibilityRole="link"
                  onPress={() => router.push(`/contacts/${person.id}`)}
                  style={[styles.person, { backgroundColor: theme.bg, borderColor: theme.border }]}
                >
                  <RecordAvatar name={contactName(person)} size={36} />
                  <View style={{ flex: 1 }}>
                    <Text style={{ color: theme.text, fontWeight: '600' }}>{contactName(person)}</Text>
                    {contactSubtitle({ title: person.title }) ? <Text style={{ color: theme.text2 }}>{person.title}</Text> : null}
                  </View>
                </Pressable>
              ))
            )}
            {people.status === 'ready' && people.hasMore ? (
              <Button label={people.loadingMore ? t('Looking…') : t('Look further')} variant="secondary" busy={people.loadingMore} onPress={() => void pager.current?.more()} />
            ) : null}
            {people.moreError ? <ErrorNotice message={people.moreError} onRetry={() => void pager.current?.more()} /> : null}
            <Button label={t('Add a person')} variant="secondary" onPress={() => router.push(`/contacts/new?company=${company.id}`)} />
          </DetailCard>
          {workspaceId ? <ActivityTimeline workspaceId={workspaceId} link={{ company_id: company.id }} /> : null}
          <Text style={{ color: theme.text2, fontSize: 13 }}>
            {t('Added {date}', { date: formatDate(new Date(company.created_at)) })} · {t('Updated {date}', { date: formatDate(new Date(company.updated_at)) })}
          </Text>
        </>
      )}
      <ConfirmDialog
        visible={confirming}
        title={t('Delete {name}?', { name: company?.name ?? t('this company') })}
        message={t('The people linked to it stay in your contacts and lose the link. Deals keep their history without the company.')}
        confirmLabel={t('Delete company')}
        busy={deleting}
        error={deleteError}
        onConfirm={() => void remove()}
        onCancel={() => setConfirming(false)}
      />
    </ScreenFrame>
  );
}

const styles = StyleSheet.create({
  hero: { flexDirection: 'row', alignItems: 'center', gap: 16 },
  title: { fontSize: 26, fontWeight: '700' },
  person: { flexDirection: 'row', alignItems: 'center', gap: 12, borderWidth: 1, borderRadius: radius, padding: 10, minHeight: 56 }
});
