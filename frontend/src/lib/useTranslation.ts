import { useCallback } from 'react'
import useLanguageStore from '../stores/languageStore'
import { translations } from '../lib/translations'

const useTranslation = () => {
  const { language } = useLanguageStore()
  
  const t = useCallback((keyPath: string, data?: Record<string, string | number>): string => {
    const keys = keyPath.split('.')
    let current: any = translations[language]
    
    for (const key of keys) {
      if (current && current[key] !== undefined) {
        current = current[key]
      } else {
        // Fallback to English if key missing in current language
        let fallback: any = translations['en']
        for (const fKey of keys) {
          if (fallback && fallback[fKey] !== undefined) {
            fallback = fallback[fKey]
          } else {
            return keyPath 
          }
        }
        current = fallback
        break
      }
    }
    
    let result = current as string
    if (data && typeof result === 'string') {
      Object.entries(data).forEach(([key, value]) => {
        result = result.replace(`{{${key}}}`, String(value))
      })
    }
    
    return result
  }, [language])

  return { t, language, setLanguage: useLanguageStore.getState().setLanguage }
}

export default useTranslation
