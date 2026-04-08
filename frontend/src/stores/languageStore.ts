import { create } from 'zustand'
import { persist } from 'zustand/middleware'

type Language = 'en' | 'id'

interface LanguageState {
  language: Language
  setLanguage: (lang: Language) => void
}

const useLanguageStore = create<LanguageState>()(
  persist(
    (set) => ({
      language: (localStorage.getItem('language') as Language) || 'id', // Default to ID as requested
      setLanguage: (lang) => set({ language: lang }),
    }),
    {
      name: 'language-storage',
    }
  )
)

export default useLanguageStore
