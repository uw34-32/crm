// Creates or edits a deal. Creating also picks the pipeline and whether the deal is in
// progress or already won; an existing deal changes stage only by moving, so editing
// leaves them out. Checks mirror the API's limits and run on submit.

import { useRef, useState } from 'react';
import { Pressable, Text, View } from 'react-native';
import { toSentence } from '../lib/forms';
import { dayKey } from '../lib/activities/timeline';
import { validateDeal, type DealDraft } from '../lib/deals/fields';
import { majorText } from '../lib/deals/money';
import { Button } from './Button';
import { ErrorNotice } from './ErrorNotice';
import { Field, type TextInputHandle } from './Field';
import { MemberPicker } from './MemberPicker';
import { OptionChips } from './OptionChips';
import { RecordPicker, type Choice } from './RecordPicker';
import { useTheme } from '../theme';
import { nameOf, t } from '../i18n';

type Initial = {
  title?: string;
  currency?: string;
  value_minor?: number;
  pipeline_id?: string;
  expected_close_date?: string | null;
  owner_id?: string | null;
  company?: Choice | null;
  contact?: Choice | null;
};

type Props = {
  workspaceId: string;
  initial?: Initial;
  pipelines?: { id: string; name: string }[];
  creating?: boolean;
  submitLabel: string;
  busy: boolean;
  error: string;
  onSubmit: (draft: DealDraft) => void;
  onCancel: () => void;
};

export function DealForm({ workspaceId, initial = {}, pipelines: choices = [], creating = false, submitLabel, busy, error, onSubmit, onCancel }: Props) {
  const theme = useTheme();
  const [title, setTitle] = useState(initial.title ?? '');
  const [currency, setCurrency] = useState(initial.currency ?? 'USD');
  const [valueTouched, setValueTouched] = useState(false);
  const [value, setValue] = useState(initial.value_minor ? majorText(initial.value_minor, initial.currency ?? 'USD') : '');
  const [pipelineId, setPipelineId] = useState(initial.pipeline_id ?? choices[0]?.id ?? '');
  // A new deal asks only what recording a sale needs; the rest opens on request.
  const [details, setDetails] = useState(false);
  const showDetails = !creating || details;
  const [status, setStatus] = useState<'open' | 'won'>('won');
  // The date is the day the sale was made, so it only suits a won deal: an open one has no date until one is typed.
  function changeStatus(next: 'open' | 'won') {
    setStatus(next);
    if (!dateTouched) setDate(next === 'won' ? dayKey(new Date()) : '');
  }
  // A new deal records a sale made today unless the date is changed; an edited one keeps its own.
  const [dateTouched, setDateTouched] = useState(false);
  const [date, setDate] = useState(initial.expected_close_date ?? (creating ? dayKey(new Date()) : ''));
  const [owner, setOwner] = useState(initial.owner_id ?? '');
  const [company, setCompany] = useState<Choice | null>(initial.company ?? null);
  const [contact, setContact] = useState<Choice | null>(initial.contact ?? null);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const valueInput = useRef<TextInputHandle>(null);
  const currencyInput = useRef<TextInputHandle>(null);
  const dateInput = useRef<TextInputHandle>(null);

  // The stored amount is in minor units, so an untouched value follows the currency's decimals.
  function changeCurrency(text: string) {
    setCurrency(text);
    if (!valueTouched && initial.value_minor) setValue(majorText(initial.value_minor, text.trim().toUpperCase()));
  }

  function submit() {
    const draft: DealDraft = {
      title,
      value,
      currency,
      pipeline_id: pipelineId,
      status,
      expected_close_date: date,
      company_id: company?.id ?? null,
      contact_id: contact?.id ?? null,
      owner_id: owner || null
    };
    const found = validateDeal(creating ? draft : { ...draft, pipeline_id: pipelineId || 'existing' });
    setErrors(found);
    // The pipeline sits under More details, so an error on it opens them.
    if (found.pipeline_id) setDetails(true);
    if (Object.keys(found).length === 0) onSubmit(draft);
  }

  return (
    <View style={{ gap: 16 }}>
      {error ? <ErrorNotice message={toSentence(error)} /> : null}
      <Field label={t('Title')} value={title} onChangeText={setTitle} error={errors.title} returnKeyType="next" onSubmitEditing={() => valueInput.current?.focus()} />
      <Field ref={valueInput} label={t('Value')} value={value} onChangeText={(text) => { setValueTouched(true); setValue(text); }} error={errors.value} keyboardType="decimal-pad" placeholder={t('1250.50')} returnKeyType="next" onSubmitEditing={() => currencyInput.current?.focus()} />
      <Field ref={currencyInput} label={t('Currency')} value={currency} onChangeText={changeCurrency} error={errors.currency} autoCapitalize="characters" autoCorrect={false} maxLength={3} returnKeyType="next" onSubmitEditing={() => dateInput.current?.focus()} />
      {creating ? (
        <OptionChips
          label={t('Status')}
          value={status}
          options={[{ value: 'open', label: t('In progress') }, { value: 'won', label: nameOf('Won') }]}
          onChange={changeStatus}
        />
      ) : null}
      {creating ? <Text style={{ color: theme.text2 }}>{t('Won records a sale that is already closed. In progress keeps the deal open on the board.')}</Text> : null}
      {errors.pipeline_id ? <ErrorNotice message={errors.pipeline_id} /> : null}
      <Field ref={dateInput} label={t('Expected close date')} value={date} onChangeText={(text) => { setDateTouched(true); setDate(text); }} error={errors.expected_close_date} placeholder="2027-03-14" autoCapitalize="none" autoCorrect={false} returnKeyType="done" />
      {creating ? (
        <Pressable accessibilityRole="button" accessibilityState={{ expanded: details }} onPress={() => setDetails((open) => !open)} hitSlop={8}>
          <Text style={{ color: theme.accentText, fontSize: 16 }}>{details ? t('Fewer details') : t('More details')}</Text>
        </Pressable>
      ) : null}
      {showDetails ? (
        <>
        {creating && choices.length > 1 ? (
          <OptionChips label={t('Pipeline')} value={pipelineId} options={choices.map((p) => ({ value: p.id, label: nameOf(p.name) }))} onChange={setPipelineId} />
        ) : null}
        <RecordPicker kind="companies" workspaceId={workspaceId} value={company} onChange={setCompany} />
        <RecordPicker kind="contacts" workspaceId={workspaceId} value={contact} onChange={setContact} />
        <MemberPicker label={t('Owner')} workspaceId={workspaceId} value={owner} onChange={setOwner} />
        </>
      ) : null}
      <Button label={submitLabel} busy={busy} onPress={submit} />
      <Button label={t('Cancel')} variant="secondary" disabled={busy} onPress={onCancel} />
    </View>
  );
}
