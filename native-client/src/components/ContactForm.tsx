import { useRef, useState } from 'react';
import { View } from 'react-native';
import { toSentence } from '../lib/forms';
import { validateContact, type ContactDraft } from '../lib/records/fields';
import { Button } from './Button';
import { RecordPicker, type Choice } from './RecordPicker';
import { ErrorNotice } from './ErrorNotice';
import { Field, type TextInputHandle } from './Field';
import { t } from '../i18n';

type Props = {
  workspaceId: string;
  initial?: { first_name?: string; last_name?: string; email?: string; phone?: string; title?: string; company?: Choice | null };
  submitLabel: string;
  busy: boolean;
  error: string;
  onSubmit: (draft: ContactDraft) => void;
  onCancel: () => void;
};

export function ContactForm({ workspaceId, initial = {}, submitLabel, busy, error, onSubmit, onCancel }: Props) {
  const [first, setFirst] = useState(initial.first_name ?? '');
  const [last, setLast] = useState(initial.last_name ?? '');
  const [email, setEmail] = useState(initial.email ?? '');
  const [phone, setPhone] = useState(initial.phone ?? '');
  const [title, setTitle] = useState(initial.title ?? '');
  const [company, setCompany] = useState<Choice | null>(initial.company ?? null);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const lastInput = useRef<TextInputHandle>(null);
  const emailInput = useRef<TextInputHandle>(null);
  const phoneInput = useRef<TextInputHandle>(null);
  const titleInput = useRef<TextInputHandle>(null);

  function submit() {
    const draft: ContactDraft = { first_name: first, last_name: last, email, phone, title, company_id: company?.id ?? null };
    const found = validateContact(draft);
    setErrors(found);
    if (Object.keys(found).length === 0) onSubmit(draft);
  }

  return (
    <View style={{ gap: 16 }}>
      {error ? <ErrorNotice message={toSentence(error)} /> : null}
      <Field label={t('First name')} value={first} onChangeText={setFirst} error={errors.first_name} autoComplete="given-name" returnKeyType="next" onSubmitEditing={() => lastInput.current?.focus()} />
      <Field ref={lastInput} label={t('Last name')} value={last} onChangeText={setLast} error={errors.last_name} autoComplete="family-name" returnKeyType="next" onSubmitEditing={() => emailInput.current?.focus()} />
      <Field ref={emailInput} label={t('Email')} value={email} onChangeText={setEmail} error={errors.email} autoCapitalize="none" autoComplete="email" keyboardType="email-address" returnKeyType="next" onSubmitEditing={() => phoneInput.current?.focus()} />
      <Field ref={phoneInput} label={t('Phone')} value={phone} onChangeText={setPhone} error={errors.phone} autoComplete="tel" keyboardType="phone-pad" returnKeyType="next" onSubmitEditing={() => titleInput.current?.focus()} />
      <Field ref={titleInput} label={t('Job title')} value={title} onChangeText={setTitle} error={errors.title} returnKeyType="done" />
      <RecordPicker workspaceId={workspaceId} value={company} onChange={setCompany} />
      <Button label={submitLabel} busy={busy} onPress={submit} />
      <Button label={t('Cancel')} variant="secondary" disabled={busy} onPress={onCancel} />
    </View>
  );
}
