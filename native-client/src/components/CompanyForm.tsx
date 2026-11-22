import { useRef, useState } from 'react';
import { View } from 'react-native';
import { toSentence } from '../lib/forms';
import { validateCompany, type CompanyDraft } from '../lib/records/fields';
import { Button } from './Button';
import { ErrorNotice } from './ErrorNotice';
import { Field, type TextInputHandle } from './Field';
import { t } from '../i18n';

type Props = {
  initial?: CompanyDraft;
  submitLabel: string;
  busy: boolean;
  error: string;
  onSubmit: (draft: CompanyDraft) => void;
  onCancel: () => void;
};

export function CompanyForm({ initial = {}, submitLabel, busy, error, onSubmit, onCancel }: Props) {
  const [name, setName] = useState(initial.name ?? '');
  const [domain, setDomain] = useState(initial.domain ?? '');
  const [phone, setPhone] = useState(initial.phone ?? '');
  const [industry, setIndustry] = useState(initial.industry ?? '');
  const [address, setAddress] = useState(initial.address ?? '');
  const [errors, setErrors] = useState<Record<string, string>>({});
  const domainInput = useRef<TextInputHandle>(null);
  const phoneInput = useRef<TextInputHandle>(null);
  const industryInput = useRef<TextInputHandle>(null);
  const addressInput = useRef<TextInputHandle>(null);

  function submit() {
    const draft: CompanyDraft = { name, domain, phone, industry, address };
    const found = validateCompany(draft);
    setErrors(found);
    if (Object.keys(found).length === 0) onSubmit(draft);
  }

  return (
    <View style={{ gap: 16 }}>
      {error ? <ErrorNotice message={toSentence(error)} /> : null}
      <Field label={t('Name')} value={name} onChangeText={setName} error={errors.name} returnKeyType="next" onSubmitEditing={() => domainInput.current?.focus()} />
      <Field ref={domainInput} label={t('Domain')} value={domain} onChangeText={setDomain} error={errors.domain} placeholder={t('acme.example')} autoCapitalize="none" autoCorrect={false} keyboardType="url" returnKeyType="next" onSubmitEditing={() => phoneInput.current?.focus()} />
      <Field ref={phoneInput} label={t('Phone')} value={phone} onChangeText={setPhone} error={errors.phone} autoComplete="tel" keyboardType="phone-pad" returnKeyType="next" onSubmitEditing={() => industryInput.current?.focus()} />
      <Field ref={industryInput} label={t('Industry')} value={industry} onChangeText={setIndustry} error={errors.industry} returnKeyType="next" onSubmitEditing={() => addressInput.current?.focus()} />
      <Field ref={addressInput} label={t('Address')} value={address} onChangeText={setAddress} error={errors.address} multiline returnKeyType="done" />
      <Button label={submitLabel} busy={busy} onPress={submit} />
      <Button label={t('Cancel')} variant="secondary" disabled={busy} onPress={onCancel} />
    </View>
  );
}
