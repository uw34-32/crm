import { useRef, useState } from 'react';
import { Text } from 'react-native';
import { Link } from 'expo-router';
import { AuthScreen } from '../../components/AuthScreen';
import { Button } from '../../components/Button';
import { ErrorNotice } from '../../components/ErrorNotice';
import { Field, type TextInputHandle } from '../../components/Field';
import { checkEmail, checkPassword, toSentence } from '../../lib/forms';
import { useStore } from '../../lib/store';
import { session } from '../../services';
import { useTheme } from '../../theme';
import { t, useLocale } from '../../i18n';

export default function Login() {
  useLocale();
  const theme = useTheme();
  const { ended } = useStore(session.state);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [errors, setErrors] = useState({ email: '', password: '' });
  const [formError, setFormError] = useState('');
  const [busy, setBusy] = useState(false);
  const passwordInput = useRef<TextInputHandle>(null);

  async function submit() {
    setFormError('');
    const next = { email: checkEmail(email), password: checkPassword(password, { creating: false }) };
    setErrors(next);
    if (next.email || next.password) return;
    setBusy(true);
    try {
      await session.signIn({ email: email.trim(), password });
    } catch (err) {
      setFormError(toSentence(err instanceof Error ? err.message : String(err)));
      setBusy(false);
    }
  }

  return (
    <AuthScreen
      title={t('Sign in')}
      footer={
        <Text style={{ color: theme.text2 }}>
          {t('New here?')} <Link href="/register" style={{ color: theme.accentText }}>{t('Create an account')}</Link>
        </Text>
      }
    >
      {ended && !formError ? <Text style={{ color: theme.text2 }}>{t('Your session has ended. Sign in again to continue.')}</Text> : null}
      {formError ? <ErrorNotice message={formError} /> : null}
      <Field
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
        autoComplete="current-password"
        textContentType="password"
        returnKeyType="go"
        onSubmitEditing={() => void submit()}
      />
      <Button label={busy ? t('Signing in…') : t('Sign in')} busy={busy} onPress={() => void submit()} />
    </AuthScreen>
  );
}
