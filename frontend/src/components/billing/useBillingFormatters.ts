import { useCallback, useMemo } from 'react'

import { toMajorUnits } from '@/lib/billing-ui'
import useTranslation from '@/lib/useTranslation'

export function useBillingFormatters() {
  const { t, language } = useTranslation()
  const locale = language === 'id' ? 'id-ID' : 'en-US'
  const numberFormat = useMemo(() => new Intl.NumberFormat(locale), [locale])
  const dateFormat = useMemo(() => new Intl.DateTimeFormat(locale, { dateStyle: 'medium' }), [locale])
  const formatNumber = useCallback((value: number) => numberFormat.format(value), [numberFormat])
  const formatCredits = formatNumber
  const formatDate = useCallback(
    (value?: string) => (value ? dateFormat.format(new Date(value)) : '—'),
    [dateFormat],
  )
  const formatMoney = useMemo(() => {
    const cache = new Map<string, Intl.NumberFormat>()
    return (amountMinor: number, currency: string) => {
      let formatter = cache.get(currency)
      if (!formatter) {
        formatter = new Intl.NumberFormat(locale, { style: 'currency', currency })
        cache.set(currency, formatter)
      }
      return formatter.format(toMajorUnits(amountMinor, currency))
    }
  }, [locale])
  const formatStatus = useCallback(
    (status: string) => {
      const translated = t(`billing.statuses.${status}`)
      if (translated !== `billing.statuses.${status}`) return translated
      const topup = t(`billing.topupStatuses.${status}`)
      if (topup !== `billing.topupStatuses.${status}`) return topup
      return status.replace(/_/g, ' ')
    },
    [t],
  )
  const statusVariant = useCallback(
    (status: string): 'secondary' | 'destructive' | 'outline' =>
      status === 'paid' || status === 'active'
        ? 'secondary'
        : status === 'suspended' || status === 'payment_due'
          ? 'destructive'
          : 'outline',
    [],
  )
  const translateLedgerType = useCallback(
    (type: string) => {
      const key = `billing.ledgerTypes.${type}`
      const translated = t(key)
      return translated === key ? type.replace(/_/g, ' ') : translated
    },
    [t],
  )
  const translateResourceType = useCallback(
    (type: string) => {
      const key = `billing.resourceTypes.${type}`
      const translated = t(key)
      return translated === key ? type : translated
    },
    [t],
  )
  const formatResourceDisplayName = useCallback(
    (name?: string, type?: string) => {
      const cleanName = name?.trim()
      if (cleanName) return cleanName
      return t('billing.unnamedService', { type: translateResourceType(type || 'project') })
    },
    [t, translateResourceType],
  )

  return {
    t,
    language,
    formatNumber,
    formatCredits,
    formatDate,
    formatMoney,
    formatStatus,
    statusVariant,
    translateLedgerType,
    formatResourceDisplayName,
  }
}
