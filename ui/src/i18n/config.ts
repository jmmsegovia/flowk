import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import en from './en.json';
import es from './es.json';

const resources = {
  en: { translation: en },
  es: { translation: es }
};

const FALLBACK_LANGUAGE = 'en';

const loadInitialLanguage = (): string => {
  if (typeof window === 'undefined') {
    return FALLBACK_LANGUAGE;
  }
  const stored = window.localStorage.getItem('appLanguage');
  if (stored && resources[stored as keyof typeof resources]) {
    return stored;
  }
  return FALLBACK_LANGUAGE;
};

i18n.use(initReactI18next).init({
  resources,
  lng: loadInitialLanguage(),
  fallbackLng: FALLBACK_LANGUAGE,
  interpolation: {
    escapeValue: false
  }
});

export default i18n;
