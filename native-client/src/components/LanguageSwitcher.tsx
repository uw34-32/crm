// The language of the screens, each named in its own language so it can be found from the
// other. The choice is kept on the device.

import { LOCALES, LOCALE_NAMES, setLocale, t, useLocale, type Locale } from '../i18n';
import { OptionChips } from './OptionChips';

export function LanguageSwitcher() {
  const locale = useLocale();
  return <OptionChips<Locale> label={t('Language')} value={locale} options={LOCALES.map((code) => ({ value: code, label: LOCALE_NAMES[code] }))} onChange={(code) => void setLocale(code)} />;
}
