import { useCallback } from 'react'
import useLanguageStore from '../stores/languageStore'
import { translations } from '../lib/translations'

const useTranslation = () => {
  const { language } = useLanguageStore()
  
  const t = useCallback((keyPath: string, data?: Record<string, string | number>): any => {
    const keys = keyPath.split('.')
    let current: unknown = translations[language as keyof typeof translations]
    
    for (const key of keys) {
      if (current && typeof current === 'object' && (current as Record<string, unknown>)[key] !== undefined) {
        current = (current as Record<string, unknown>)[key]
      } else {
        // Fallback to English if key missing in current language
        let fallback: unknown = translations['en']
        for (const fKey of keys) {
          if (fallback && typeof fallback === 'object' && (fallback as Record<string, unknown>)[fKey] !== undefined) {
            fallback = (fallback as Record<string, unknown>)[fKey]
          } else {
            return keyPath 
          }
        }
        current = fallback
        break
      }
    }
    
    if (current && typeof current === 'object') {
      return current
    }
    
    let result = String(current)
    if (data && typeof result === 'string') {
      Object.entries(data).forEach(([key, value]) => {
        result = result.replace(`{{${key}}}`, String(value))
      })
    }
    
    return result
  }, [language])

  return { t, language, setLanguage: (lang: string) => useLanguageStore.getState().setLanguage(lang as 'en' | 'id') }
}

export default useTranslation
