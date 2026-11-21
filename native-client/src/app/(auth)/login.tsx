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

export default function Login() {
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
      title="Sign in"
      footer={
        <Text style={{ color: theme.text2 }}>
          New here? <Link href="/register" style={{ color: theme.accentText }}>Create an account</Link>
        </Text>
      }
    >
      {ended && !formError ? <Text style={{ color: theme.text2 }}>Your session has ended. Sign in again to continue.</Text> : null}
      {formError ? <ErrorNotice message={formError} /> : null}
      <Field
        label="Email"
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
        label="Password"
        value={password}
        onChangeText={setPassword}
        error={errors.password}
        secureTextEntry
        autoComplete="current-password"
        textContentType="password"
        returnKeyType="go"
        onSubmitEditing={() => void submit()}
      />
      <Button label={busy ? 'Signing in…' : 'Sign in'} busy={busy} onPress={() => void submit()} />
    </AuthScreen>
  );
}
