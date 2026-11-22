import { useRef, useState } from 'react';
import { Text } from 'react-native';
import { Link } from 'expo-router';
import { AuthScreen } from '../../components/AuthScreen';
import { Button } from '../../components/Button';
import { ErrorNotice } from '../../components/ErrorNotice';
import { Field, type TextInputHandle } from '../../components/Field';
import { checkEmail, checkName, checkPassword, toSentence } from '../../lib/forms';
import { session } from '../../services';
import { useTheme } from '../../theme';
import { t, useLocale } from '../../i18n';

export default function Register() {
  useLocale();
  const theme = useTheme();
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [errors, setErrors] = useState({ name: '', email: '', password: '' });
  const [formError, setFormError] = useState('');
  const [busy, setBusy] = useState(false);
  const emailInput = useRef<TextInputHandle>(null);
  const passwordInput = useRef<TextInputHandle>(null);

  async function submit() {
    setFormError('');
    const next = {
      name: checkName(name, t('Enter your name.')),
      email: checkEmail(email),
      password: checkPassword(password, { creating: true })
    };
    setErrors(next);
    if (next.name || next.email || next.password) return;
    setBusy(true);
    try {
      await session.register({ name: name.trim(), email: email.trim(), password });
    } catch (err) {
      setFormError(toSentence(err instanceof Error ? err.message : String(err)));
      setBusy(false);
    }
  }

  return (
    <AuthScreen
      title={t('Create an account')}
      footer={
        <Text style={{ color: theme.text2 }}>
          {t('Already have an account?')} <Link href="/login" style={{ color: theme.accentText }}>{t('Sign in')}</Link>
        </Text>
      }
    >
      {formError ? <ErrorNotice message={formError} /> : null}
      <Field
        label={t('Name')}
        value={name}
        onChangeText={setName}
        error={errors.name}
        autoComplete="name"
        textContentType="name"
        returnKeyType="next"
        onSubmitEditing={() => emailInput.current?.focus()}
      />
      <Field
        ref={emailInput}
        label={t('Email')}
        value={email}
        onChangeText={setEmail}
        error={errors.email}
        autoCapitalize="none"
        autoComplete="email"
        keyboardType="email-address"
        textContentType="username"
        returnKeyType="next"
        onSubmitEditing={() => passwordInput.current?.focus()}
      />
      <Field
        ref={passwordInput}
        label={t('Password')}
        value={password}
        onChangeText={setPassword}
        error={errors.password}
        secureTextEntry
        autoComplete="new-password"
        textContentType="newPassword"
        returnKeyType="go"
        onSubmitEditing={() => void submit()}
      />
      <Button label={busy ? t('Creating account…') : t('Create account')} busy={busy} onPress={() => void submit()} />
    </AuthScreen>
  );
}
