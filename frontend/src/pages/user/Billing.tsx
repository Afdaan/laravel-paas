import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  AlertTriangle,
  ArrowDownRight,
  ArrowUpRight,
  Building,
  Building2,
  Calendar,
  CalendarClock,
  Check,
  CheckCircle2,
  Coins,
  Copy,
  CreditCard,
  Database,
  FileText,
  FolderGit2,
  Hash,
  Mail,
  MapPin,
  Phone,
  PlusCircle,
  Printer,
  ReceiptText,
  RefreshCw,
  Search,
  WalletCards,
} from 'lucide-react'
import { motion } from 'framer-motion'
import { toast } from 'sonner'
import { QRCodeSVG } from 'qrcode.react'
import { billingAPI } from '@/services/api'
import axios from 'axios'
import {
  createTopupIdempotencyKey,
  hasLowCreditBalance,
  nextBillingRequestState,
  toMajorUnits,
  type BillingRequestState,
} from '@/lib/billing-ui'
import { usePolling } from '@/lib/usePolling'
import useTranslation from '@/lib/useTranslation'
import type { BillingOverview, BillingStatus, TopupPackage, BillingProfile, TopupResponse } from '@/types'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Switch } from '@/components/ui/switch'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

export function isValidPhoneNumber(phone: string, country: string = 'ID'): boolean {
  const trimmed = phone.trim()
  if (!trimmed) return false

  const digitsOnly = trimmed.replace(/\D/g, '')

  if (country === 'ID' || country === 'IDN') {
    if (digitsOnly.startsWith('08')) {
      return digitsOnly.length >= 10 && digitsOnly.length <= 13
    }
    if (digitsOnly.startsWith('628')) {
      return digitsOnly.length >= 11 && digitsOnly.length <= 14
    }
    if (digitsOnly.startsWith('8')) {
      return digitsOnly.length >= 9 && digitsOnly.length <= 12
    }
    return false
  }

  return digitsOnly.length >= 7 && digitsOnly.length <= 15
}

export default function Billing() {
  const [profile, setProfile] = useState<BillingProfile>({
    company_name: '', tax_id: '', email: '', phone: '',
    address_line1: '', address_line2: '', city: '', state_province: '',
    postal_code: '', country: 'ID'
  })
  const [savingProfile, setSavingProfile] = useState(false)
  const [isProfileSaved, setIsProfileSaved] = useState(false)
  const [activePaymentModal, setActivePaymentModal] = useState<TopupResponse | null>(null)
  const [checkingPaymentStatus, setCheckingPaymentStatus] = useState(false)

  const { t, language } = useTranslation()
  const [overview, setOverview] = useState<BillingRequestState<BillingOverview>>({ status: 'idle' })
  const [packages, setPackages] = useState<BillingRequestState<TopupPackage[]>>({ status: 'idle' })
  const [statuses, setStatuses] = useState<BillingRequestState<BillingStatus[]>>({ status: 'idle' })
  const [renewLoading, setRenewLoading] = useState<Record<string, boolean>>({})
  const [pendingRenewChange, setPendingRenewChange] = useState<{
    resource_id: number
    resource_type: 'project' | 'database'
    resource_name: string
    target_auto_renew: boolean
  } | null>(null)
  const [topupPackageID, setTopupPackageID] = useState<number | null>(null)
  const [customAmount, setCustomAmount] = useState('')
  const [staleWarning, setStaleWarning] = useState(false)
  const [showProfilePrompt, setShowProfilePrompt] = useState(false)
  const topupKeys = useRef<Map<number, string>>(new Map())
  const customTopupKey = useRef<string | null>(null)
  const loadInFlight = useRef(false)
  const hasLoadedProfileRef = useRef(false)

  const [selectedInvoice, setSelectedInvoice] = useState<BillingOverview['invoices'][0] | null>(null)
  const [copiedInvoiceID, setCopiedInvoiceID] = useState<number | null>(null)
  const [invoiceSearch, setInvoiceSearch] = useState('')
  const [invoiceStatusFilter, setInvoiceStatusFilter] = useState<'all' | 'paid' | 'payment_due'>('all')

  const [touchedFields, setTouchedFields] = useState<Record<string, boolean>>({})
  const markTouched = (field: string) => setTouchedFields((prev) => ({ ...prev, [field]: true }))

  const getInvoiceNumber = useCallback((inv: { id: number; period_start?: string; created_at?: string }) => {
    const dateObj = new Date(inv.period_start || inv.created_at || Date.now())
    const year = dateObj.getFullYear() || 2026
    const month = String(dateObj.getMonth() + 1).padStart(2, '0')
    return `INV-${year}${month}-${String(inv.id).padStart(4, '0')}`
  }, [])

  const handleCopyInvoiceNumber = (invoiceNumber: string, id: number) => {
    void navigator.clipboard.writeText(invoiceNumber)
    setCopiedInvoiceID(id)
    toast.success(t('billing.copiedInvoice'))
    setTimeout(() => setCopiedInvoiceID(null), 2000)
  }

  const profileValidation = useMemo(() => {
    const companyNameClean = profile.company_name.trim()
    const emailClean = profile.email.trim()
    const addressClean = profile.address_line1.trim()
    const cityClean = profile.city.trim()
    const postalCodeClean = profile.postal_code.trim()
    const taxIdClean = profile.tax_id.replace(/\D/g, '')

    const isCompanyNameValid = companyNameClean.length >= 2
    const isEmailValid = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(emailClean)
    const isPhoneValid = isValidPhoneNumber(profile.phone, profile.country)
    const isAddressValid = addressClean.length >= 5
    const isCityValid = cityClean.length >= 2
    const isPostalCodeValid = /^[0-9]{5}$/.test(postalCodeClean)
    const isTaxIdValid = !profile.tax_id.trim() || taxIdClean.length >= 15

    const isValid =
      isCompanyNameValid &&
      isEmailValid &&
      isPhoneValid &&
      isAddressValid &&
      isCityValid &&
      isPostalCodeValid &&
      isTaxIdValid

    return {
      isValid,
      isCompanyNameValid,
      isEmailValid,
      isPhoneValid,
      isAddressValid,
      isCityValid,
      isPostalCodeValid,
      isTaxIdValid,
    }
  }, [profile])

  const isProfileComplete = useCallback((prof: BillingProfile) => {
    const companyNameClean = prof.company_name.trim()
    const emailClean = prof.email.trim()
    const addressClean = prof.address_line1.trim()
    const cityClean = prof.city.trim()
    const postalCodeClean = prof.postal_code.trim()

    return (
      companyNameClean.length >= 2 &&
      /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(emailClean) &&
      isValidPhoneNumber(prof.phone, prof.country) &&
      addressClean.length >= 5 &&
      cityClean.length >= 2 &&
      /^[0-9]{5}$/.test(postalCodeClean)
    )
  }, [])

  const scrollToBillingProfile = () => {
    const el = document.getElementById('billing-profile-card')
    if (el && typeof el.scrollIntoView === 'function') {
      el.scrollIntoView({ behavior: 'smooth' })
    }
  }

  const checkProfileAndPrompt = (): boolean => {
    if (!isProfileSaved && !isProfileComplete(profile)) {
      toast.error(t('billing.profile.profileRequired'))
      setShowProfilePrompt(true)
      scrollToBillingProfile()
      return false
    }
    return true
  }
  const overviewRef = useRef(overview)
  const packagesRef = useRef(packages)
  const statusesRef = useRef(statuses)

  useEffect(() => {
    overviewRef.current = overview
    packagesRef.current = packages
    statusesRef.current = statuses
  })

  const formatNumber = useCallback(
    (value: number) => new Intl.NumberFormat(language === 'id' ? 'id-ID' : 'en-US').format(value),
    [language],
  )
  const formatCredits = useCallback((credits: number) => formatNumber(credits), [formatNumber])
  const formatDate = useCallback(
    (value?: string) =>
      value
        ? new Intl.DateTimeFormat(language === 'id' ? 'id-ID' : 'en-US', { dateStyle: 'medium' }).format(
            new Date(value),
          )
        : '—',
    [language],
  )
  const formatMoney = useCallback(
    (amountMinor: number, currency: string) =>
      new Intl.NumberFormat(language === 'id' ? 'id-ID' : 'en-US', {
        style: 'currency',
        currency,
      }).format(toMajorUnits(amountMinor, currency)),
    [language],
  )
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
    (status: string) =>
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

  const filteredInvoices = useMemo(() => {
    if (overview.status !== 'success') return []
    return overview.data.invoices.filter((invoice) => {
      const invNumber = getInvoiceNumber(invoice).toLowerCase()
      const searchClean = invoiceSearch.toLowerCase().trim()
      const matchesSearch =
        !searchClean ||
        invNumber.includes(searchClean) ||
        invoice.id.toString().includes(searchClean)
      const matchesStatus =
        invoiceStatusFilter === 'all' || invoice.status === invoiceStatusFilter
      return matchesSearch && matchesStatus
    })
  }, [overview, invoiceSearch, invoiceStatusFilter, getInvoiceNumber])

  const invoiceMetrics = useMemo(() => {
    if (overview.status !== 'success') {
      return { totalCreditsInvoiced: 0, count: 0, latestPaidInvoice: null }
    }
    const totalCredits = overview.data.invoices.reduce((acc, inv) => acc + (inv.total_credits || 0), 0)
    const paidInvoices = overview.data.invoices.filter((inv) => inv.status === 'paid')
    return {
      totalCreditsInvoiced: totalCredits,
      count: overview.data.invoices.length,
      latestPaidInvoice: paidInvoices[0] || null,
    }
  }, [overview])

  const didLoadInitial = useRef(false)

  const load = useCallback(async () => {
    if (loadInFlight.current) return
    loadInFlight.current = true

    const markLoading = <T,>(setter: React.Dispatch<React.SetStateAction<BillingRequestState<T>>>) => {
      setter((current) => (current.status === 'success' ? current : { status: 'loading' }))
    }

    markLoading(setOverview)
    markLoading(setPackages)
    markLoading(setStatuses)

    const currentOverview = overview
    const currentPackages = packages
    const currentStatuses = statuses

    const [overviewResult, catalogResult, statusResult] = await Promise.allSettled([
      billingAPI.overview(),
      billingAPI.catalog(),
      billingAPI.status(),
    ])

    const nextOverview = nextBillingRequestState(overviewResult, currentOverview, (response) => response.data)
    const nextPackages = nextBillingRequestState(catalogResult, currentPackages, (response) => response.data.packages)
    const nextStatuses = nextBillingRequestState(statusResult, currentStatuses, (response) => response.data)
    setOverview(nextOverview)
    setPackages(nextPackages)
    setStatuses(nextStatuses)
    const hasStaleData =
      (overviewResult.status === 'rejected' && nextOverview.status === 'success') ||
      (catalogResult.status === 'rejected' && nextPackages.status === 'success') ||
      (statusResult.status === 'rejected' && nextStatuses.status === 'success')
    setStaleWarning(hasStaleData)

    if (!hasLoadedProfileRef.current) {
      hasLoadedProfileRef.current = true
      try {
        const resProf = await billingAPI.getProfile()
        if (resProf.data) {
          const loadedProf = {
            company_name: resProf.data.company_name ?? '',
            tax_id: resProf.data.tax_id ?? '',
            email: resProf.data.email ?? '',
            phone: resProf.data.phone ?? '',
            address_line1: resProf.data.address_line1 ?? '',
            address_line2: resProf.data.address_line2 ?? '',
            city: resProf.data.city ?? '',
            state_province: resProf.data.state_province ?? '',
            postal_code: resProf.data.postal_code ?? '',
            country: resProf.data.country || 'ID',
          }
          setProfile(loadedProf)
          if (isProfileComplete(loadedProf)) {
            setIsProfileSaved(true)
          }
        }
      } catch (e) {
        // ignore error (e.g. initial profile not created or backend unavailable)
      }
    }
    loadInFlight.current = false
  }, [overview, packages, statuses])

  useEffect(() => {
    if (didLoadInitial.current) return
    didLoadInitial.current = true

    const params = new URLSearchParams(window.location.search)
    if (params.get('payment_return') !== 'pakasir') {
      void load()
      return
    }

    const rawTopupRef = params.get('topup_ref') ?? ''
    const rawTopupID = params.get('topup_id') ?? ''
    params.delete('payment_return')
    params.delete('topup_ref')
    params.delete('topup_id')
    const remainingQuery = params.toString()
    window.history.replaceState(
      window.history.state,
      '',
      `${window.location.pathname}${remainingQuery ? `?${remainingQuery}` : ''}${window.location.hash}`,
    )

    const topupRef = rawTopupRef.trim()
    const topupID = /^\d+$/.test(rawTopupID) ? Number(rawTopupID) : 0
    if (!topupRef && (!Number.isSafeInteger(topupID) || topupID <= 0)) {
      void load()
      return
    }

    void (async () => {
      try {
        const response = topupRef
          ? await billingAPI.reconcileTopupByRef(topupRef)
          : await billingAPI.reconcileTopup(topupID)
        if (response.data.status === 'paid') {
          toast.success('Pembayaran Berhasil! Wallet Anda telah terisi.')
        } else {
          toast.info('Pembayaran masih diproses. Status akan diperbarui otomatis.')
        }
      } catch {
        toast.error('Gagal memverifikasi pembayaran Pakasir')
      } finally {
        await load()
      }
    })()
  }, [load])

  usePolling(() => void load(), 30_000)

  const attentionResources = useMemo(() => {
    if (statuses.status !== 'success') return []
    return statuses.data.filter(({ status }) => status === 'payment_due' || status === 'suspended')
  }, [statuses])

  const startTopup = async (packageID: number) => {
    if (!checkProfileAndPrompt()) return
    setTopupPackageID(packageID)
    try {
      const idempotencyKey = topupKeys.current.get(packageID) ?? createTopupIdempotencyKey()
      topupKeys.current.set(packageID, idempotencyKey)
      const response = await billingAPI.createTopup(packageID, idempotencyKey)
            if (response.data.payment_token && response.data.payment_token.startsWith('000201')) {
        topupKeys.current.delete(packageID)
        setActivePaymentModal(response.data)
        return
      }
      if (!response.data.payment_url) {
        topupKeys.current.delete(packageID)
        throw new Error(t('billing.paymentSessionUnavailable'))
      }
      topupKeys.current.delete(packageID)
      window.location.assign(response.data.payment_url)
    } catch (error) {
      if (axios.isAxiosError(error) && error.response) {
        topupKeys.current.delete(packageID)
      }
      toast.error(t('billing.topupStartFailed'))
    } finally {
      setTopupPackageID(null)
    }
  }

  const customAmountNum = parseInt(customAmount.replace(/\D/g, ''), 10) || 0
  const customCredits = Math.floor(customAmountNum / 1000)
  const customValid = customAmountNum >= 10_000 && customAmountNum <= 10_000_000 && customAmountNum % 1000 === 0

  const handleCustomAmountChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    customTopupKey.current = null
    const raw = e.target.value.replace(/\D/g, '')
    if (!raw) {
      setCustomAmount('')
      return
    }
    const val = parseInt(raw, 10)
    if (isNaN(val)) {
      setCustomAmount('')
      return
    }
    setCustomAmount(val.toLocaleString(language === 'id' ? 'id-ID' : 'en-US'))
  }

  const startCustomTopup = async () => {
    if (!checkProfileAndPrompt()) return
    setTopupPackageID(-1)
    try {
      const idempotencyKey = customTopupKey.current ?? createTopupIdempotencyKey()
      customTopupKey.current = idempotencyKey
      const response = await billingAPI.createTopup(0, idempotencyKey, customAmountNum)
      if (response.data.payment_token && response.data.payment_token.startsWith('000201')) {
        customTopupKey.current = null
        setActivePaymentModal(response.data)
        return
      }
      if (!response.data.payment_url) {
        customTopupKey.current = null
        throw new Error(t('billing.paymentSessionUnavailable'))
      }
      customTopupKey.current = null
      setCustomAmount('')
      window.location.assign(response.data.payment_url)
    } catch (error) {
      if (axios.isAxiosError(error) && error.response) {
        customTopupKey.current = null
      }
      toast.error(t('billing.topupStartFailed'))
    } finally {
      setTopupPackageID(null)
    }
  }

  const reconcileTopup = async (topupID: number) => {
    try {
      await billingAPI.reconcileTopup(topupID)
      await load()
      toast.success(t('billing.statusRefreshed'))
    } catch {
      toast.error(t('billing.statusRefreshFailed'))
    }
  }

  const balanceData = overview.status === 'success' ? overview.data.wallet.balance_credits : null
  const upcomingCredits = overview.status === 'success' ? overview.data.upcoming_required_credits : null
  const activePaymentURL = activePaymentModal?.payment_url
  const showLowBalance =
    balanceData !== null &&
    upcomingCredits !== null &&
    hasLowCreditBalance(balanceData, upcomingCredits) &&
    attentionResources.length === 0


  const handleSaveProfile = async (e: React.FormEvent) => {
    e.preventDefault()
    setSavingProfile(true)
    try {
      const res = await billingAPI.updateProfile(profile)
      if (res.data) {
        hasLoadedProfileRef.current = true
        setIsProfileSaved(true)
        setProfile({
          company_name: res.data.company_name ?? profile.company_name,
          tax_id: res.data.tax_id ?? profile.tax_id,
          email: res.data.email ?? profile.email,
          phone: res.data.phone ?? profile.phone,
          address_line1: res.data.address_line1 ?? profile.address_line1,
          address_line2: res.data.address_line2 ?? profile.address_line2,
          city: res.data.city ?? profile.city,
          state_province: res.data.state_province ?? profile.state_province,
          postal_code: res.data.postal_code ?? profile.postal_code,
          country: res.data.country || profile.country || 'ID',
        })
      } else {
        setIsProfileSaved(true)
      }
      toast.success(t('billing.profile.saved'))
    } catch {
      toast.error(t('billing.profile.saveFailed'))
    } finally {
      setSavingProfile(false)
    }
  }

  const handleCheckStatusModal = async () => {
    if (!activePaymentModal) return
    setCheckingPaymentStatus(true)
    try {
      await billingAPI.reconcileTopup(activePaymentModal.id)
      await load()
      const overviewRes = await billingAPI.overview()
      const currentTopup = overviewRes.data.topups.find((t: any) => t.id === activePaymentModal.id)
      if (currentTopup && currentTopup.status === 'paid') {
        toast.success('Pembayaran Berhasil! Wallet Anda telah terisi.')
        setActivePaymentModal(null)
      } else {
        toast.info('Pembayaran belum diterima. Silakan selesaikan pembayaran.')
      }
    } catch (e) {
      toast.error('Gagal mengecek status pembayaran')
    } finally {
      setCheckingPaymentStatus(false)
    }
  }

  return (
    <div className="mx-auto max-w-6xl space-y-8 pb-12 animate-in fade-in duration-500">
      {/* Header section with breadcrumbs */}
      <div className="flex flex-col gap-4 border-b border-border/60 pb-6 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <div className="flex items-center gap-1.5 text-xs text-muted-foreground font-medium mb-1">
            <span>Dashboard</span>
            <span className="text-muted-foreground/60">/</span>
            <span className="text-foreground font-semibold">{t('billing.nav')}</span>
          </div>
          <h1 className="text-2xl font-bold tracking-tight text-foreground sm:text-3xl">{t('billing.title')}</h1>
          <p className="mt-1 text-xs text-muted-foreground max-w-2xl">{t('billing.description')}</p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => void load()}
          disabled={loadInFlight.current}
          className="self-start h-8 px-3 text-xs shadow-xs transition-all hover:bg-muted sm:self-auto"
        >
          <RefreshCw className={`mr-1.5 size-3.5 ${loadInFlight.current ? 'animate-spin' : ''}`} />
          {t('billing.refresh')}
        </Button>
      </div>

      {/* Warnings & Alerts */}
      {staleWarning && (
        <Card className="border-amber-500/40 bg-amber-500/10 shadow-xs" role="status" aria-live="polite">
          <CardContent className="flex items-start gap-3 p-4 text-sm text-amber-800 dark:text-amber-300">
            <RefreshCw className="mt-0.5 size-5 shrink-0 animate-spin text-amber-600 dark:text-amber-400" />
            <div>
              <strong>{t('billing.staleData')}</strong>
            </div>
          </CardContent>
        </Card>
      )}

      {attentionResources.length > 0 && (
        <Card className="border-destructive/40 bg-destructive/10 shadow-xs" role="alert" aria-live="assertive">
          <CardContent className="flex items-start gap-3 p-4 text-sm text-destructive dark:text-red-400">
            <AlertTriangle className="mt-0.5 size-5 shrink-0 text-destructive" />
            <div>
              <strong className="font-bold">{t('billing.paymentRequired')}</strong>{' '}
              {attentionResources
                .map((resource) =>
                  resource.oldest_due_at
                    ? `${translateResourceType(resource.resource_type)} #${resource.resource_id}, ${t('billing.dueOn', { date: formatDate(resource.oldest_due_at) })} (${formatStatus(resource.status)})`
                    : `${translateResourceType(resource.resource_type)} #${resource.resource_id} (${formatStatus(resource.status)})`,
                )
                .join('; ')}{' '}
              {t('billing.paymentRequiredDescription')}
            </div>
          </CardContent>
        </Card>
      )}

      {showLowBalance && (
        <Card className="border-amber-500/40 bg-gradient-to-r from-amber-500/10 via-amber-500/5 to-transparent shadow-xs" role="status">
          <CardContent className="flex items-start gap-3 p-4 text-sm text-amber-800 dark:text-amber-300">
            <AlertTriangle className="mt-0.5 size-5 shrink-0 text-amber-600 dark:text-amber-400" />
            <div>
              <strong className="font-bold">{t('billing.lowBalance')}</strong> {t('billing.lowBalanceDescription')}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Main Billing Overview Card */}
      <Card className="overflow-hidden border-border/60 shadow-xs transition-all hover:border-border/80">
        <CardHeader className="bg-muted/20 border-b border-border/40 pb-4">
          <div>
            <CardTitle className="text-lg font-bold flex items-center gap-2 text-foreground">
              <WalletCards className="size-4.5 text-primary" />
              {t('billing.creditsOverview')}
            </CardTitle>
            <CardDescription className="mt-0.5 text-xs text-muted-foreground">{t('billing.balanceDescription')}</CardDescription>
          </div>
        </CardHeader>
        <CardContent className="space-y-6 pt-6">
          {/* Stat Cards (Double-Bezel Hardware Architecture) */}
          <div className="grid gap-4 sm:grid-cols-2">
            {overview.status === 'loading' &&
              Array.from({ length: 2 }).map((_, index) => <Skeleton key={index} className="h-28 rounded-xl" />)}
            {overview.status === 'error' && (
              <div className="col-span-full rounded-xl border border-destructive/30 bg-destructive/5 p-4 text-center text-sm text-destructive">
                {t('billing.unavailable')}
              </div>
            )}
            {overview.status === 'success' && (
              <>
                {/* Balance Card - Double Bezel Shell */}
                <div className="group relative overflow-hidden rounded-2xl border border-primary/20 bg-primary/5 p-1.5 transition-all hover:border-primary/40">
                  <div className="rounded-xl border border-primary/10 bg-background/80 p-5 backdrop-blur-sm">
                    <div className="flex items-center justify-between">
                      <p className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground">{t('billing.balance')}</p>
                      <div className="flex size-7 items-center justify-center rounded-lg bg-primary/10 text-primary">
                        <WalletCards className="size-3.5" />
                      </div>
                    </div>
                    <div className="mt-3 flex items-baseline gap-2">
                      <p 
                        className="text-3xl font-bold tracking-tight text-foreground tabular-nums"
                        title={`${formatNumber(overview.data.wallet.balance_credits)} Credits`}
                      >
                        {formatCredits(overview.data.wallet.balance_credits)}
                        <span className="ml-2 font-sans text-xs font-semibold uppercase tracking-widest text-muted-foreground">{t('billing.credits')}</span>
                      </p>
                    </div>
                  </div>
                </div>

                {/* Upcoming Charges Card - Double Bezel Shell */}
                <div className="group relative overflow-hidden rounded-2xl border border-border/50 bg-muted/20 p-1.5 transition-all hover:border-border">
                  <div className="rounded-xl border border-border/40 bg-card/90 p-5">
                    <div className="flex items-center justify-between">
                      <p className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground">{t('billing.upcomingCharges')}</p>
                      <div className="flex size-7 items-center justify-center rounded-lg bg-blue-500/10 text-blue-500">
                        <CalendarClock className="size-3.5" />
                      </div>
                    </div>
                    <div className="mt-3 flex items-baseline gap-2">
                      <p 
                        className="text-3xl font-bold tracking-tight text-foreground tabular-nums"
                        title={`${formatNumber(overview.data.upcoming_required_credits)} Credits`}
                      >
                        {formatCredits(overview.data.upcoming_required_credits)}
                        <span className="ml-2 font-sans text-xs font-semibold uppercase tracking-widest text-muted-foreground">{t('billing.credits')}</span>
                      </p>
                    </div>
                  </div>
                </div>
              </>
            )}
          </div>

          {/* Add Credits Packages Section */}
          <div className="pt-2">
            <div className="mb-4 flex items-center justify-between">
              <div>
                <h3 className="text-sm font-bold text-foreground flex items-center gap-2">
                  <PlusCircle className="size-4 text-primary" />
                  {t('billing.addCredits')}
                </h3>
                <p className="mt-0.5 max-w-2xl text-xs leading-relaxed text-muted-foreground">{t('billing.addCreditsDescription')}</p>
              </div>
            </div>

            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
              {packages.status === 'loading' &&
                Array.from({ length: 4 }).map((_, index) => <Skeleton key={index} className="h-36 rounded-xl" />)}
              {packages.status === 'error' && (
                <p className="col-span-full text-sm text-destructive">{t('billing.catalogLoadFailed')}</p>
              )}
              {packages.status === 'success' &&
                packages.data.map((pkg, idx) => {
                  const isPopular = idx === 1 || packages.data.length === 1
                  return (
                    <motion.button
                      key={pkg.id}
                      type="button"
                      initial={{ opacity: 0, y: 8 }}
                      animate={{ opacity: 1, y: 0 }}
                      transition={{ type: 'spring', stiffness: 120, damping: 20, delay: idx * 0.05 }}
                      className={`group relative flex flex-col rounded-xl border p-5 text-left transition-all duration-200 hover:-translate-y-0.5 hover:shadow-sm active:scale-[0.98] disabled:opacity-60 ${
                        isPopular
                          ? 'border-primary/40 bg-primary/[0.04] shadow-xs hover:border-primary/60'
                          : 'border-border/60 bg-card hover:border-border/80'
                      }`}
                      disabled={topupPackageID !== null}
                      onClick={() => void startTopup(pkg.id)}
                    >
                      {isPopular && (
                        <span className="absolute -top-2.5 right-3 rounded-full bg-primary/15 px-2.5 py-0.5 text-[10px] font-bold uppercase tracking-wider text-primary ring-1 ring-primary/30">
                          {t('billing.bestValue')}
                        </span>
                      )}

                      <div>
                        <div className="text-2xl font-bold tracking-tight text-foreground tabular-nums">
                          {formatCredits(pkg.credits)}
                        </div>
                        <div className="mt-0.5 text-xs font-medium text-muted-foreground">{t('billing.credits')}</div>
                      </div>

                      <div className="mt-auto flex items-center justify-between border-t border-border/40 pt-3 mt-4">
                        <span className="text-xs font-bold text-foreground">{formatMoney(pkg.amount_minor, pkg.currency)}</span>
                        <span className="flex items-center gap-1 text-xs font-medium text-primary">
                          {topupPackageID === pkg.id ? t('billing.openingCheckout') : t('billing.choosePackage')}
                          <ArrowUpRight className="size-3 transition-transform duration-200 group-hover:translate-x-0.5 group-hover:-translate-y-0.5" />
                        </span>
                      </div>
                    </motion.button>
                  )
                })}
              {packages.status === 'success' && packages.data.length === 0 && (
                <p className="col-span-full rounded-xl border border-dashed p-6 text-center text-sm text-muted-foreground">
                  {t('billing.noPackages')}
                </p>
              )}
            </div>

            {/* Custom Amount Box */}
            <div className="mt-5 rounded-xl border border-border/60 bg-muted/20 p-4">
              <label htmlFor="custom-amount" className="mb-2.5 flex items-center gap-2 text-xs font-semibold text-foreground">
                <Coins className="size-3.5 text-muted-foreground" />
                {t('billing.customTopupLabel')}
              </label>
              <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
                <div className="relative flex-1">
                  <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-xs font-medium text-muted-foreground">Rp</span>
                  <Input
                    id="custom-amount"
                    type="text"
                    inputMode="numeric"
                    placeholder="50.000"
                    className="pl-9 text-xs tabular-nums h-9"
                    value={customAmount}
                    onChange={handleCustomAmountChange}
                    disabled={topupPackageID !== null}
                  />
                </div>
                {customAmountNum > 0 && customValid && (
                  <span className="shrink-0 text-xs font-medium text-muted-foreground">
                    = {customCredits.toLocaleString(language)} {t('billing.credits')}
                  </span>
                )}
                <Button
                  size="sm"
                  disabled={!customValid || topupPackageID !== null}
                  onClick={() => void startCustomTopup()}
                  className="shrink-0 h-9 px-4 text-xs font-medium"
                >
                  {topupPackageID === -1 ? t('billing.openingCheckout') : t('billing.customTopupButton')}
                </Button>
              </div>
              {customAmount && !customValid && (
                <p className="mt-2 text-xs text-muted-foreground">{t('billing.customTopupHint')}</p>
              )}
            </div>
          </div>
        </CardContent>
      </Card>

      <Card className="border-border/60 shadow-sm">
        <CardHeader className="pb-4">
          <CardTitle className="flex items-center gap-2 text-xl font-bold">
            <CalendarClock className="size-5 text-primary" />
            {t('billing.resourceBilling')}
          </CardTitle>
          <CardDescription>{t('billing.resourceBillingDescription')}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {overview.status === 'loading' && <Skeleton className="h-24 rounded-xl" />}
          {overview.status === 'error' && <p className="text-sm text-muted-foreground">{t('billing.unavailable')}</p>}
          {overview.status === 'success' && overview.data.resources.length === 0 && (
            <p className="text-sm text-muted-foreground">{t('billing.noBillableResources')}</p>
          )}
          {overview.status === 'success' &&
            overview.data.resources.map((resource) => (
              <div
                key={`${resource.resource_type}-${resource.resource_id}`}
                className="flex flex-col gap-4 rounded-xl border border-border/60 bg-muted/20 p-4 sm:flex-row sm:items-center sm:justify-between"
              >
                <div className="min-w-0 space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge
                      variant="outline"
                      className="bg-background/80 font-mono text-[10px] uppercase tracking-wider text-muted-foreground"
                    >
                      {resource.resource_type === 'project' ? (
                        <>
                          <FolderGit2 className="mr-1 size-3 text-cyan-500" />
                          {t('billing.resourceTypes.project')}
                        </>
                      ) : (
                        <>
                          <Database className="mr-1 size-3 text-purple-500" />
                          {t('billing.resourceTypes.database')}
                        </>
                      )}
                    </Badge>
                    <p className="truncate font-semibold text-foreground">
                      {resource.resource_name || `${translateResourceType(resource.resource_type)} #${resource.resource_id}`}
                    </p>
                    <Badge variant={statusVariant(resource.status)}>{formatStatus(resource.status)}</Badge>
                  </div>
                  <p className="text-sm text-muted-foreground">
                    {resource.spec_name} · {formatCredits(resource.monthly_credits)} {t('billing.credits')} / {t('billing.month')}
                    {resource.resource_type === 'project' && resource.cpu_millicores && resource.memory_mb ? (
                      <span className="ml-1 text-xs text-muted-foreground/80">
                        ({resource.cpu_millicores}m CPU · {resource.memory_mb} MB RAM)
                      </span>
                    ) : null}
                    {resource.resource_type === 'database' && (resource.engine || resource.storage_gb) ? (
                      <span className="ml-1 text-xs text-muted-foreground/80">
                        ({[resource.engine?.toUpperCase(), resource.storage_gb ? `${resource.storage_gb} GB` : null]
                          .filter(Boolean)
                          .join(' · ')})
                      </span>
                    ) : null}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {t('billing.currentPeriod', {
                      start: formatDate(resource.current_period_start),
                      end: formatDate(resource.next_invoice_at),
                    })}
                  </p>
                </div>
                <div className="flex flex-col gap-2 sm:items-end">
                  <p className="text-sm font-medium text-foreground">
                    {resource.status === 'active'
                      ? t('billing.renewsOn', { date: formatDate(resource.next_invoice_at) })
                      : t('billing.renewalPaymentDue', { date: formatDate(resource.next_invoice_at) })}
                  </p>
                  <div className="flex items-center gap-2">
                    <span className="text-xs text-muted-foreground">{t('billing.autoRenew')}</span>
                    <Switch
                      id={`auto-renew-${resource.resource_type}-${resource.resource_id}`}
                      checked={resource.auto_renew}
                      disabled={renewLoading[`${resource.resource_type}-${resource.resource_id}`]}
                      onCheckedChange={(checked) => {
                        setPendingRenewChange({
                          resource_id: resource.resource_id,
                          resource_type: resource.resource_type,
                          resource_name:
                            resource.resource_name ||
                            `${translateResourceType(resource.resource_type)} #${resource.resource_id}`,
                          target_auto_renew: checked,
                        })
                      }}
                    />
                  </div>
                </div>
              </div>
            ))}
        </CardContent>
      </Card>

      {/* Billing & Wallet History Section */}

      {/* Auto-Renew Confirmation Modal */}
      <Dialog open={pendingRenewChange !== null} onOpenChange={(open) => !open && setPendingRenewChange(null)}>
        <DialogContent className="sm:max-w-[425px]">
          <DialogHeader>
            <DialogTitle>
              {pendingRenewChange?.target_auto_renew
                ? t('billing.enableAutoRenewTitle')
                : t('billing.disableAutoRenewTitle')}
            </DialogTitle>
            <DialogDescription className="pt-2 leading-relaxed">
              {pendingRenewChange?.target_auto_renew
                ? t('billing.enableAutoRenewDescription', { name: pendingRenewChange?.resource_name ?? '' })
                : t('billing.disableAutoRenewDescription', { name: pendingRenewChange?.resource_name ?? '' })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="mt-4 gap-2 sm:gap-0">
            <Button variant="outline" onClick={() => setPendingRenewChange(null)}>
              {t('billing.cancel')}
            </Button>
            <Button
              onClick={async () => {
                if (!pendingRenewChange) return
                const { resource_id, resource_type, target_auto_renew } = pendingRenewChange
                const key = `${resource_type}-${resource_id}`
                setPendingRenewChange(null)
                setRenewLoading((prev) => ({ ...prev, [key]: true }))
                try {
                  await billingAPI.updateAutoRenew(resource_id, resource_type, target_auto_renew)
                  await load()
                  toast.success(
                    target_auto_renew ? t('billing.autoRenewEnabled') : t('billing.autoRenewDisabled'),
                  )
                } catch (error) {
                  if (axios.isAxiosError(error)) {
                    const status = error.response?.status
                    const serverMessage = error.response?.data?.error || error.response?.data?.message
                    if (status === 429) {
                      toast.error(serverMessage || t('billing.autoRenewRateLimited'))
                    } else if (serverMessage) {
                      toast.error(serverMessage)
                    } else {
                      toast.error(t('billing.autoRenewFailed'))
                    }
                  } else {
                    toast.error(t('billing.autoRenewFailed'))
                  }
                } finally {
                  setRenewLoading((prev) => ({ ...prev, [key]: false }))
                }
              }}
            >
              {t('billing.confirmChange')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      {/* Billing & Wallet History Section - Full Width Tabbed Data Tables */}
      <Card className="border-border/60 shadow-xs">
        <CardHeader className="border-b border-border/40 pb-4">
          <CardTitle className="text-lg font-bold flex items-center gap-2 text-foreground">
            <ReceiptText className="size-4.5 text-primary" />
            {t('billing.history')}
          </CardTitle>
          <CardDescription className="text-xs text-muted-foreground">
            Complete transaction ledger for invoices, credit top-ups, and balance adjustments.
          </CardDescription>
        </CardHeader>

        <CardContent className="pt-6">
          <Tabs defaultValue="wallet_activity" className="space-y-4">
            <TabsList className="grid w-full grid-cols-3 max-w-md h-9 text-xs">
              <TabsTrigger value="wallet_activity" className="text-xs">
                <WalletCards className="size-3.5 mr-1.5" />
                {t('billing.walletActivity')}
              </TabsTrigger>
              <TabsTrigger value="invoices" className="text-xs">
                <ReceiptText className="size-3.5 mr-1.5" />
                {t('billing.invoices')}
              </TabsTrigger>
              <TabsTrigger value="topups" className="text-xs">
                <CreditCard className="size-3.5 mr-1.5" />
                {t('billing.topups')}
              </TabsTrigger>
            </TabsList>

            {/* Wallet Activity Tab */}
            <TabsContent value="wallet_activity" className="space-y-4">
              <div className="rounded-xl border border-border/60 overflow-hidden bg-card/50">
                <Table>
                  <TableHeader className="bg-muted/40">
                    <TableRow className="border-b border-border/40 hover:bg-transparent">
                      <TableHead className="w-[32%] text-[11px] font-bold uppercase tracking-wider text-muted-foreground/80 pl-4">Transaction Type</TableHead>
                      <TableHead className="w-[24%] text-[11px] font-bold uppercase tracking-wider text-muted-foreground/80">Amount</TableHead>
                      <TableHead className="w-[24%] text-[11px] font-bold uppercase tracking-wider text-muted-foreground/80">Balance After</TableHead>
                      <TableHead className="w-[20%] text-[11px] font-bold uppercase tracking-wider text-muted-foreground/80 text-right pr-4">Date</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {overview.status === 'loading' && (
                      <TableRow>
                        <TableCell colSpan={4} className="text-center py-6 text-xs text-muted-foreground">Loading wallet history...</TableCell>
                      </TableRow>
                    )}
                    {overview.status === 'success' && overview.data.wallet.ledger_entries.length === 0 && (
                      <TableRow>
                        <TableCell colSpan={4} className="text-center py-8 text-xs text-muted-foreground">{t('billing.noWalletActivity')}</TableCell>
                      </TableRow>
                    )}
                    {overview.status === 'success' && overview.data.wallet.ledger_entries.map((entry, index) => {
                      const isCredit = entry.amount_credits >= 0
                      return (
                        <TableRow key={index} className="hover:bg-muted/30 transition-colors border-b border-border/30 last:border-0">
                          <TableCell className="font-semibold text-xs text-foreground pl-4">
                            <div className="flex items-center gap-2.5">
                              <div className={`size-6 rounded-full flex items-center justify-center shrink-0 ${
                                isCredit
                                  ? 'bg-emerald-500/10 border border-emerald-500/20 text-emerald-500'
                                  : 'bg-amber-500/10 border border-amber-500/20 text-amber-500'
                              }`}>
                                {isCredit ? <ArrowUpRight className="size-3.5" /> : <ArrowDownRight className="size-3.5" />}
                              </div>
                              <span className="capitalize font-medium text-xs">{translateLedgerType(entry.type)}</span>
                            </div>
                          </TableCell>
                          <TableCell className="text-xs">
                            <span className={`inline-flex items-center font-bold text-[11px] tabular-nums px-2.5 py-0.5 rounded-full ${
                              isCredit
                                ? 'text-emerald-600 dark:text-emerald-400 bg-emerald-500/10 border border-emerald-500/20'
                                : 'text-amber-600 dark:text-amber-400 bg-amber-500/10 border border-amber-500/20'
                            }`}>
                              {isCredit ? `+${formatCredits(entry.amount_credits)}` : formatCredits(entry.amount_credits)} {t('billing.credits')}
                            </span>
                          </TableCell>
                          <TableCell className="text-xs font-semibold tabular-nums text-foreground/80">
                            {formatCredits(entry.balance_after)} {t('billing.credits')}
                          </TableCell>
                          <TableCell className="text-xs text-muted-foreground tabular-nums text-right pr-4">
                            {formatDate(entry.created_at)}
                          </TableCell>
                        </TableRow>
                      )
                    })}
                  </TableBody>
                </Table>
              </div>
            </TabsContent>

            {/* Invoices Tab */}
            <TabsContent value="invoices" className="space-y-4">
              {/* Financial Quick Metrics */}
              {overview.status === 'success' && overview.data.invoices.length > 0 && (
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
                  <div className="rounded-xl border border-border/60 bg-muted/20 p-3.5 flex flex-col justify-between">
                    <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                      {t('billing.totalInvoiced')}
                    </span>
                    <div className="mt-1.5 flex items-baseline gap-2">
                      <span className="text-xl font-bold tracking-tight text-foreground tabular-nums">
                        {formatCredits(invoiceMetrics.totalCreditsInvoiced)}
                      </span>
                      <span className="text-[10px] font-medium text-muted-foreground uppercase">{t('billing.credits')}</span>
                    </div>
                    <span className="text-[11px] text-muted-foreground font-mono tabular-nums mt-0.5">
                      ≈ Rp {(invoiceMetrics.totalCreditsInvoiced * 1000).toLocaleString(language === 'id' ? 'id-ID' : 'en-US')}
                    </span>
                  </div>

                  <div className="rounded-xl border border-border/60 bg-muted/20 p-3.5 flex flex-col justify-between">
                    <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                      {t('billing.activeSubscriptions')}
                    </span>
                    <div className="mt-1.5 flex items-baseline gap-2">
                      <span className="text-xl font-bold tracking-tight text-foreground tabular-nums">
                        {overview.data.resources.length}
                      </span>
                      <span className="text-[10px] font-medium text-muted-foreground">{t('billing.activeServicesCount', { count: overview.data.resources.length })}</span>
                    </div>
                    <span className="text-[11px] text-muted-foreground mt-0.5">
                      Auto-renew: {overview.data.resources.filter((r) => r.auto_renew).length}
                    </span>
                  </div>

                  <div className="rounded-xl border border-border/60 bg-muted/20 p-3.5 flex flex-col justify-between">
                    <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                      {t('billing.upcomingCharges')}
                    </span>
                    <div className="mt-1.5 flex items-baseline gap-2">
                      <span className="text-xl font-bold tracking-tight text-foreground tabular-nums">
                        {formatCredits(overview.data.upcoming_required_credits)}
                      </span>
                      <span className="text-[10px] font-medium text-muted-foreground uppercase">{t('billing.credits')}</span>
                    </div>
                    <span className="text-[11px] text-muted-foreground font-mono tabular-nums mt-0.5">
                      ≈ Rp {(overview.data.upcoming_required_credits * 1000).toLocaleString(language === 'id' ? 'id-ID' : 'en-US')} / {t('billing.month')}
                    </span>
                  </div>
                </div>
              )}

              {/* Table Controls (Search & Status Filter) */}
              {overview.status === 'success' && overview.data.invoices.length > 0 && (
                <div className="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-3 pt-1">
                  <div className="relative max-w-xs flex-1">
                    <Search className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 size-3.5 text-muted-foreground" />
                    <Input
                      placeholder={t('billing.searchInvoices')}
                      value={invoiceSearch}
                      onChange={(e) => setInvoiceSearch(e.target.value)}
                      className="pl-8 text-xs h-8 bg-card"
                    />
                  </div>

                  <div className="flex items-center gap-1 bg-muted/40 p-0.5 rounded-lg border border-border/60 text-[11px]">
                    <button
                      type="button"
                      onClick={() => setInvoiceStatusFilter('all')}
                      className={`px-2.5 py-1 rounded-md font-medium transition-all ${
                        invoiceStatusFilter === 'all'
                          ? 'bg-background text-foreground shadow-xs'
                          : 'text-muted-foreground hover:text-foreground'
                      }`}
                    >
                      {t('billing.allStatuses')}
                    </button>
                    <button
                      type="button"
                      onClick={() => setInvoiceStatusFilter('paid')}
                      className={`px-2.5 py-1 rounded-md font-medium transition-all ${
                        invoiceStatusFilter === 'paid'
                          ? 'bg-background text-emerald-600 dark:text-emerald-400 shadow-xs'
                          : 'text-muted-foreground hover:text-foreground'
                      }`}
                    >
                      {formatStatus('paid')}
                    </button>
                    <button
                      type="button"
                      onClick={() => setInvoiceStatusFilter('payment_due')}
                      className={`px-2.5 py-1 rounded-md font-medium transition-all ${
                        invoiceStatusFilter === 'payment_due'
                          ? 'bg-background text-destructive shadow-xs'
                          : 'text-muted-foreground hover:text-foreground'
                      }`}
                    >
                      {formatStatus('payment_due')}
                    </button>
                  </div>
                </div>
              )}

              {/* Invoices Table */}
              <div className="rounded-xl border border-border/60 overflow-hidden bg-card/50">
                <Table>
                  <TableHeader className="bg-muted/40">
                    <TableRow className="border-b border-border/40 hover:bg-transparent">
                      <TableHead className="w-[18%] text-[11px] font-bold uppercase tracking-wider text-muted-foreground/80 pl-4">
                        {t('billing.invoiceNumber')}
                      </TableHead>
                      <TableHead className="w-[26%] text-[11px] font-bold uppercase tracking-wider text-muted-foreground/80">
                        {t('billing.servicePeriod')}
                      </TableHead>
                      <TableHead className="w-[20%] text-[11px] font-bold uppercase tracking-wider text-muted-foreground/80">
                        {t('billing.totalCharged')}
                      </TableHead>
                      <TableHead className="w-[14%] text-[11px] font-bold uppercase tracking-wider text-muted-foreground/80">
                        {t('billing.invoiceStatus')}
                      </TableHead>
                      <TableHead className="w-[12%] text-[11px] font-bold uppercase tracking-wider text-muted-foreground/80">
                        {t('billing.periodLabel')}
                      </TableHead>
                      <TableHead className="w-[10%] text-[11px] font-bold uppercase tracking-wider text-muted-foreground/80 text-right pr-4">
                        {t('billing.viewReceipt')}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {overview.status === 'loading' && (
                      <TableRow>
                        <TableCell colSpan={6} className="text-center py-6 text-xs text-muted-foreground">Loading invoices...</TableCell>
                      </TableRow>
                    )}
                    {overview.status === 'success' && overview.data.invoices.length === 0 && (
                      <TableRow>
                        <TableCell colSpan={6} className="text-center py-8 text-xs text-muted-foreground">{t('billing.noInvoices')}</TableCell>
                      </TableRow>
                    )}
                    {overview.status === 'success' && overview.data.invoices.length > 0 && filteredInvoices.length === 0 && (
                      <TableRow>
                        <TableCell colSpan={6} className="text-center py-8 text-xs text-muted-foreground">
                          {t('billing.noMatchingInvoices')}
                        </TableCell>
                      </TableRow>
                    )}
                    {overview.status === 'success' && filteredInvoices.map((invoice) => {
                      const invNumber = getInvoiceNumber(invoice)
                      const isPaid = invoice.status === 'paid'
                      return (
                        <TableRow key={invoice.id} className="hover:bg-muted/30 transition-colors border-b border-border/30 last:border-0 group">
                          {/* Invoice Number */}
                          <TableCell className="font-mono text-xs font-bold text-foreground pl-4">
                            <div className="flex items-center gap-1.5">
                              <ReceiptText className="size-3.5 text-primary shrink-0" />
                              <span className="truncate">{invNumber}</span>
                              <Button
                                variant="ghost"
                                size="icon"
                                className="size-5 text-muted-foreground/60 hover:text-foreground opacity-0 group-hover:opacity-100 transition-opacity"
                                onClick={() => handleCopyInvoiceNumber(invNumber, invoice.id)}
                                title="Salin nomor invoice"
                              >
                                {copiedInvoiceID === invoice.id ? (
                                  <Check className="size-2.5 text-emerald-500" />
                                ) : (
                                  <Copy className="size-2.5" />
                                )}
                              </Button>
                            </div>
                          </TableCell>

                          {/* Period */}
                          <TableCell className="text-xs text-foreground">
                            <div className="flex items-center gap-1.5">
                              <Calendar className="size-3 text-muted-foreground shrink-0" />
                              <span className="font-medium text-xs">
                                {formatDate(invoice.period_start)} – {formatDate(invoice.period_end)}
                              </span>
                            </div>
                          </TableCell>

                          {/* Credits Charged & IDR Equivalent */}
                          <TableCell className="text-xs font-semibold tabular-nums">
                            <div>
                              <span className="font-bold text-foreground">
                                {formatCredits(invoice.total_credits)} {t('billing.credits')}
                              </span>
                              <span className="ml-1 text-[11px] text-muted-foreground font-normal">
                                (Rp {(invoice.total_credits * 1000).toLocaleString(language === 'id' ? 'id-ID' : 'en-US')})
                              </span>
                            </div>
                          </TableCell>

                          {/* Status */}
                          <TableCell className="text-xs">
                            <Badge
                              variant={statusVariant(invoice.status)}
                              className={`capitalize font-medium text-[11px] px-2.5 py-0.5 inline-flex items-center gap-1.5 ${
                                isPaid
                                  ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border-emerald-500/20'
                                  : ''
                              }`}
                            >
                              <span className={`size-1.5 rounded-full shrink-0 ${isPaid ? 'bg-emerald-500' : 'bg-destructive'}`} />
                              {formatStatus(invoice.status)}
                            </Badge>
                          </TableCell>

                          {/* Date */}
                          <TableCell className="text-xs text-muted-foreground tabular-nums">
                            {invoice.paid_at
                              ? t('billing.paidOn', { date: formatDate(invoice.paid_at) })
                              : invoice.due_at
                                ? t('billing.dueOn', { date: formatDate(invoice.due_at) })
                                : formatDate(invoice.created_at)}
                          </TableCell>

                          {/* Action Button */}
                          <TableCell className="text-xs text-right pr-4">
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => setSelectedInvoice(invoice)}
                              className="h-7 px-2.5 text-[11px] font-semibold gap-1 text-foreground hover:bg-muted/80 shadow-2xs"
                            >
                              <ReceiptText className="size-3 text-primary" />
                              {t('billing.viewReceipt')}
                            </Button>
                          </TableCell>
                        </TableRow>
                      )
                    })}
                  </TableBody>
                </Table>
              </div>
            </TabsContent>

            {/* Top-ups Tab */}
            <TabsContent value="topups" className="space-y-4">
              <div className="rounded-xl border border-border/60 overflow-hidden bg-card/50">
                <Table>
                  <TableHeader className="bg-muted/40">
                    <TableRow className="border-b border-border/40 hover:bg-transparent">
                      <TableHead className="w-[18%] text-[11px] font-bold uppercase tracking-wider text-muted-foreground/80 pl-4">Order ID</TableHead>
                      <TableHead className="w-[20%] text-[11px] font-bold uppercase tracking-wider text-muted-foreground/80">Credits Purchased</TableHead>
                      <TableHead className="w-[20%] text-[11px] font-bold uppercase tracking-wider text-muted-foreground/80">Amount Paid</TableHead>
                      <TableHead className="w-[18%] text-[11px] font-bold uppercase tracking-wider text-muted-foreground/80">Status</TableHead>
                      <TableHead className="w-[24%] text-[11px] font-bold uppercase tracking-wider text-muted-foreground/80 text-right pr-4">Date</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {overview.status === 'loading' && (
                      <TableRow>
                        <TableCell colSpan={5} className="text-center py-6 text-xs text-muted-foreground">Loading top-ups...</TableCell>
                      </TableRow>
                    )}
                    {overview.status === 'success' && overview.data.topups.length === 0 && (
                      <TableRow>
                        <TableCell colSpan={5} className="text-center py-8 text-xs text-muted-foreground">{t('billing.noTopups')}</TableCell>
                      </TableRow>
                    )}
                    {overview.status === 'success' && overview.data.topups.map((topup) => (
                      <TableRow key={topup.id} className="hover:bg-muted/30 transition-colors border-b border-border/30 last:border-0">
                        <TableCell className="font-mono text-xs font-semibold text-foreground/90 pl-4">
                          #topup-{topup.id}
                        </TableCell>
                        <TableCell className="text-xs">
                          <span className="inline-flex items-center font-bold text-[11px] tabular-nums px-2.5 py-0.5 rounded-full text-emerald-600 dark:text-emerald-400 bg-emerald-500/10 border border-emerald-500/20">
                            +{formatCredits(topup.credits)} {t('billing.credits')}
                          </span>
                        </TableCell>
                        <TableCell className="text-xs font-bold text-foreground tabular-nums">
                          {formatMoney(topup.amount_minor, topup.currency)}
                        </TableCell>
                        <TableCell className="text-xs">
                          <Badge variant={statusVariant(topup.status)} className="capitalize font-medium text-[11px] px-2.5 py-0.5">{formatStatus(topup.status)}</Badge>
                        </TableCell>
                        <TableCell className="text-xs text-muted-foreground tabular-nums text-right pr-4">
                          <div className="flex items-center justify-end gap-2.5">
                            <span>{topup.paid_at ? t('billing.paidOn', { date: formatDate(topup.paid_at) }) : formatDate(topup.created_at)}</span>
                            {topup.status === 'pending' && (
                              <Button
                                size="sm"
                                variant="outline"
                                onClick={() => void reconcileTopup(topup.id)}
                                className="h-7 px-2.5 text-[11px] font-semibold"
                              >
                                {t('billing.checkStatus')}
                              </Button>
                            )}
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            </TabsContent>
          </Tabs>
        </CardContent>
      </Card>

      {/* Billing Profile & Tax Details - Corporate Style */}
      <Card id="billing-profile-card" className="border-border/60 bg-card shadow-xs overflow-hidden scroll-mt-6">
        <CardHeader className="border-b border-border/40 bg-muted/20 pb-5">
          <div>
            <CardTitle className="text-base font-bold tracking-tight flex items-center gap-2 text-foreground">
              <Building2 className="size-4.5 text-primary" />
              {t('billing.profile.title')}
            </CardTitle>
            <CardDescription className="mt-1 text-xs leading-relaxed text-muted-foreground">
              {t('billing.profile.description')}
            </CardDescription>
          </div>
        </CardHeader>
        <CardContent className="pt-6 space-y-6">
          {!isProfileSaved && !isProfileComplete(profile) && (
            <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 p-3.5 text-xs text-amber-600 dark:text-amber-400 font-medium flex items-start gap-2.5">
              <AlertTriangle className="size-4 shrink-0 text-amber-500 mt-0.5" />
              <div>
                <p className="font-semibold">{t('billing.profile.profileRequiredTitle')}</p>
                <p className="mt-0.5 text-muted-foreground">{t('billing.profile.incompleteBanner')}</p>
              </div>
            </div>
          )}

          <form onSubmit={handleSaveProfile} className="space-y-6">
            {/* Identity & Tax Identification Group */}
            <div className="space-y-4">
              <div className="border-b border-border/30 pb-2">
                <h4 className="text-xs font-bold uppercase tracking-wider text-muted-foreground flex items-center gap-1.5">
                  <Building2 className="size-3.5 text-primary" />
                  {t('billing.profile.identityTitle')}
                </h4>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                {/* Full Name / Company Name */}
                <div className="space-y-1.5">
                  <Label htmlFor="company_name" className="text-xs font-semibold text-foreground flex items-center gap-1.5">
                    {t('billing.profile.companyName')}
                  </Label>
                  <div className="relative">
                    <Building2 className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
                    <Input
                      id="company_name"
                      value={profile.company_name}
                      onBlur={() => markTouched('company_name')}
                      onChange={(e) => setProfile({ ...profile, company_name: e.target.value })}
                      placeholder={t('billing.profile.companyNamePlaceholder')}
                      className={`pl-9 text-xs h-9 ${
                        touchedFields.company_name && !profileValidation.isCompanyNameValid
                          ? 'border-destructive focus-visible:ring-destructive/20'
                          : ''
                      }`}
                    />
                  </div>
                  {touchedFields.company_name && !profileValidation.isCompanyNameValid ? (
                    <p className="text-[11px] text-destructive font-medium">{t('billing.profile.companyNameError')}</p>
                  ) : (
                    <p className="text-[11px] text-muted-foreground">{t('billing.profile.companyNameHint')}</p>
                  )}
                </div>

                {/* Tax ID / NPWP / NIK */}
                <div className="space-y-1.5">
                  <Label htmlFor="tax_id" className="text-xs font-semibold text-foreground flex items-center justify-between">
                    <span>{t('billing.profile.taxId')}</span>
                    <span className="text-[10px] font-normal text-muted-foreground">{t('billing.profile.optional')}</span>
                  </Label>
                  <div className="relative">
                    <FileText className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
                    <Input
                      id="tax_id"
                      value={profile.tax_id}
                      onBlur={() => markTouched('tax_id')}
                      onChange={(e) => setProfile({ ...profile, tax_id: e.target.value })}
                      placeholder={t('billing.profile.taxIdPlaceholder')}
                      className={`pl-9 text-xs h-9 ${
                        touchedFields.tax_id && !profileValidation.isTaxIdValid
                          ? 'border-destructive focus-visible:ring-destructive/20'
                          : ''
                      }`}
                    />
                  </div>
                  {touchedFields.tax_id && !profileValidation.isTaxIdValid ? (
                    <p className="text-[11px] text-destructive font-medium">{t('billing.profile.taxIdError')}</p>
                  ) : (
                    <p className="text-[11px] text-muted-foreground">{t('billing.profile.taxIdHint')}</p>
                  )}
                </div>
              </div>
            </div>

            {/* Contact & Receipt Delivery Group */}
            <div className="space-y-4">
              <div className="border-b border-border/30 pb-2">
                <h4 className="text-xs font-bold uppercase tracking-wider text-muted-foreground flex items-center gap-1.5">
                  <Mail className="size-3.5 text-primary" />
                  {t('billing.profile.contactTitle')}
                </h4>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                {/* Billing Email */}
                <div className="space-y-1.5">
                  <Label htmlFor="billing_email" className="text-xs font-semibold text-foreground">
                    {t('billing.profile.email')}
                  </Label>
                  <div className="relative">
                    <Mail className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
                    <Input
                      id="billing_email"
                      type="email"
                      value={profile.email}
                      onBlur={() => markTouched('email')}
                      onChange={(e) => setProfile({ ...profile, email: e.target.value })}
                      placeholder={t('billing.profile.emailPlaceholder')}
                      className={`pl-9 text-xs h-9 ${
                        touchedFields.email && !profileValidation.isEmailValid
                          ? 'border-destructive focus-visible:ring-destructive/20'
                          : ''
                      }`}
                    />
                  </div>
                  {touchedFields.email && !profileValidation.isEmailValid ? (
                    <p className="text-[11px] text-destructive font-medium">{t('billing.profile.emailError')}</p>
                  ) : (
                    <p className="text-[11px] text-muted-foreground">{t('billing.profile.emailHint')}</p>
                  )}
                </div>

                {/* Phone Number */}
                <div className="space-y-1.5">
                  <Label htmlFor="billing_phone" className="text-xs font-semibold text-foreground">
                    {t('billing.profile.phone')}
                  </Label>
                  <div className="relative">
                    <Phone className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
                    <Input
                      id="billing_phone"
                      value={profile.phone}
                      onBlur={() => markTouched('phone')}
                      onChange={(e) => setProfile({ ...profile, phone: e.target.value })}
                      placeholder={t('billing.profile.phonePlaceholder')}
                      className={`pl-9 text-xs h-9 ${
                        touchedFields.phone && !profileValidation.isPhoneValid
                          ? 'border-destructive focus-visible:ring-destructive/20'
                          : ''
                      }`}
                    />
                  </div>
                  {touchedFields.phone && !profileValidation.isPhoneValid ? (
                    <p className="text-[11px] text-destructive font-medium">{t('billing.profile.phoneError')}</p>
                  ) : (
                    <p className="text-[11px] text-muted-foreground">{t('billing.profile.phoneHint')}</p>
                  )}
                </div>
              </div>
            </div>

            {/* Address Information Group */}
            <div className="space-y-4">
              <div className="border-b border-border/30 pb-2">
                <h4 className="text-xs font-bold uppercase tracking-wider text-muted-foreground flex items-center gap-1.5">
                  <MapPin className="size-3.5 text-primary" />
                  {t('billing.profile.addressTitle')}
                </h4>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                {/* Street Address */}
                <div className="space-y-1.5 md:col-span-3">
                  <Label htmlFor="address_line1" className="text-xs font-semibold text-foreground">
                    {t('billing.profile.streetAddress')}
                  </Label>
                  <div className="relative">
                    <MapPin className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
                    <Input
                      id="address_line1"
                      value={profile.address_line1}
                      onBlur={() => markTouched('address_line1')}
                      onChange={(e) => setProfile({ ...profile, address_line1: e.target.value })}
                      placeholder={t('billing.profile.streetAddressPlaceholder')}
                      className={`pl-9 text-xs h-9 ${
                        touchedFields.address_line1 && !profileValidation.isAddressValid
                          ? 'border-destructive focus-visible:ring-destructive/20'
                          : ''
                      }`}
                    />
                  </div>
                  {touchedFields.address_line1 && !profileValidation.isAddressValid && (
                    <p className="text-[11px] text-destructive font-medium">{t('billing.profile.streetAddressError')}</p>
                  )}
                </div>

                {/* City */}
                <div className="space-y-1.5">
                  <Label htmlFor="city" className="text-xs font-semibold text-foreground">
                    {t('billing.profile.city')}
                  </Label>
                  <div className="relative">
                    <Building className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
                    <Input
                      id="city"
                      value={profile.city}
                      onBlur={() => markTouched('city')}
                      onChange={(e) => setProfile({ ...profile, city: e.target.value })}
                      placeholder={t('billing.profile.cityPlaceholder')}
                      className={`pl-9 text-xs h-9 ${
                        touchedFields.city && !profileValidation.isCityValid
                          ? 'border-destructive focus-visible:ring-destructive/20'
                          : ''
                      }`}
                    />
                  </div>
                  {touchedFields.city && !profileValidation.isCityValid && (
                    <p className="text-[11px] text-destructive font-medium">{t('billing.profile.cityError')}</p>
                  )}
                </div>

                {/* Postal Code */}
                <div className="space-y-1.5">
                  <Label htmlFor="postal_code" className="text-xs font-semibold text-foreground">
                    {t('billing.profile.postalCode')}
                  </Label>
                  <div className="relative">
                    <Hash className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
                    <Input
                      id="postal_code"
                      value={profile.postal_code}
                      onBlur={() => markTouched('postal_code')}
                      onChange={(e) => setProfile({ ...profile, postal_code: e.target.value })}
                      placeholder={t('billing.profile.postalCodePlaceholder')}
                      className={`pl-9 text-xs h-9 ${
                        touchedFields.postal_code && !profileValidation.isPostalCodeValid
                          ? 'border-destructive focus-visible:ring-destructive/20'
                          : ''
                      }`}
                    />
                  </div>
                  {touchedFields.postal_code && !profileValidation.isPostalCodeValid && (
                    <p className="text-[11px] text-destructive font-medium">{t('billing.profile.postalCodeError')}</p>
                  )}
                </div>

                {/* Country Read-only Badge */}
                <div className="space-y-1.5">
                  <Label className="text-xs font-semibold text-foreground">
                    {t('billing.profile.country')}
                  </Label>
                  <div className="h-9 px-3 flex items-center rounded-md border border-border bg-muted/30 text-xs font-medium text-foreground">
                    {t('billing.profile.countryValue')}
                  </div>
                </div>
              </div>
            </div>

            <div className="flex flex-col sm:flex-row items-center justify-between gap-3 pt-4 border-t border-border/40">
              {!profileValidation.isValid ? (
                <p className="text-[11px] text-amber-600 dark:text-amber-400 font-medium flex items-center gap-1.5">
                  <AlertTriangle className="size-3.5 shrink-0" />
                  {t('billing.profile.fillAllFieldsHint')}
                </p>
              ) : (
                <span />
              )}
              <Button
                type="submit"
                size="sm"
                disabled={!profileValidation.isValid || savingProfile}
                className="font-semibold gap-1.5 hover:-translate-y-0.5 active:scale-[0.98] transition-all disabled:opacity-50 disabled:cursor-not-allowed disabled:hover:translate-y-0"
              >
                {savingProfile ? (
                  <>
                    <RefreshCw className="size-3.5 animate-spin" />
                    {t('billing.profile.saving')}
                  </>
                ) : (
                  <>
                    <CheckCircle2 className="size-3.5" />
                    {t('billing.profile.save')}
                  </>
                )}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>

      {/* Profile Required Modal Prompt */}
      <Dialog open={showProfilePrompt} onOpenChange={setShowProfilePrompt}>
        <DialogContent className="sm:max-w-md border-border">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-base font-bold text-amber-500">
              <AlertTriangle className="size-4.5" />
              {t('billing.profile.profileRequiredTitle')}
            </DialogTitle>
            <DialogDescription className="text-xs text-muted-foreground pt-1.5 leading-relaxed">
              {t('billing.profile.profileRequiredDesc')}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="pt-2">
            <Button
              size="sm"
              onClick={() => {
                setShowProfilePrompt(false)
                scrollToBillingProfile()
              }}
              className="font-semibold text-xs"
            >
              {t('billing.profile.completeProfileBtn')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Modal Payment / QRIS Dialog */}
      <Dialog open={!!activePaymentModal} onOpenChange={(open) => !open && setActivePaymentModal(null)}>
        <DialogContent className="sm:max-w-md border-border">
          <DialogHeader>
            <DialogTitle className="text-center text-lg font-bold">Pembayaran Top-up</DialogTitle>
            <DialogDescription className="text-center text-xs text-muted-foreground">
              Silakan selesaikan pembayaran untuk mengisi kredit wallet.
            </DialogDescription>
          </DialogHeader>

          {activePaymentModal && (
            <div className="flex flex-col items-center justify-center space-y-4 py-4">
              {activePaymentModal.payment_token && activePaymentModal.payment_token.startsWith('000201') ? (
                <div className="bg-white p-4 border border-border rounded-lg flex flex-col items-center">
                  <QRCodeSVG
                    value={activePaymentModal.payment_token}
                    size={200}
                    level="M"
                    className="size-48 object-contain"
                  />
                  <p className="mt-2 font-mono text-[10px] uppercase text-muted-foreground">Scan QRIS via Mobile Banking / E-Wallet</p>
                </div>
              ) : activePaymentModal.payment_token ? (
                <div className="p-4 bg-muted rounded-md text-center">
                  <p className="text-xs text-muted-foreground uppercase font-mono mb-1">Kode / Nomor Pembayaran</p>
                  <p className="text-lg font-mono font-bold tracking-widest">{activePaymentModal.payment_token}</p>
                </div>
              ) : null}

              <div className="text-center">
                <p className="text-xs text-muted-foreground font-mono uppercase">Total Tagihan</p>
                <p className="text-xl font-bold text-foreground">
                  Rp {activePaymentModal.amount_minor.toLocaleString('id-ID')}
                </p>
              </div>
            </div>
          )}

          <DialogFooter className="flex flex-col sm:flex-row gap-2">
            {activePaymentURL && (
              <Button variant="outline" size="sm" onClick={() => window.location.assign(activePaymentURL)}>
                Buka Link Pembayaran
              </Button>
            )}
            <Button
              onClick={handleCheckStatusModal}
              disabled={checkingPaymentStatus}
              size="sm"
            >
              {checkingPaymentStatus ? 'Mengecek...' : 'Cek Status Pembayaran'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Invoice Detail / Official Tax Receipt Modal */}
      <Dialog open={selectedInvoice !== null} onOpenChange={(open) => !open && setSelectedInvoice(null)}>
        <DialogContent className="sm:max-w-2xl max-h-[90vh] overflow-y-auto p-0 border-border/80 bg-card text-foreground">
          {selectedInvoice && (
            <div>
              {/* Modal Top Bar */}
              <div className="flex items-center justify-between px-6 py-3.5 border-b border-border/60 bg-muted/30">
                <div className="flex items-center gap-2">
                  <ReceiptText className="size-4 text-primary" />
                  <span className="font-mono text-xs font-bold text-foreground">
                    {getInvoiceNumber(selectedInvoice)}
                  </span>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-6 text-muted-foreground hover:text-foreground"
                    onClick={() => handleCopyInvoiceNumber(getInvoiceNumber(selectedInvoice), selectedInvoice.id)}
                    title="Salin Nomor Invoice"
                  >
                    {copiedInvoiceID === selectedInvoice.id ? (
                      <Check className="size-3 text-emerald-500" />
                    ) : (
                      <Copy className="size-3" />
                    )}
                  </Button>
                </div>
                <div className="flex items-center gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => window.print()}
                    className="h-7 text-xs font-semibold gap-1.5"
                  >
                    <Printer className="size-3" />
                    {t('billing.printReceipt')}
                  </Button>
                </div>
              </div>

              {/* Official Printable Invoice Document */}
              <div className="p-6 space-y-6 text-foreground bg-card">
                {/* Document Header */}
                <div className="flex flex-col sm:flex-row justify-between gap-4 pb-6 border-b border-border/60">
                  <div>
                    <div className="flex items-center gap-2">
                      <div className="size-7 rounded-lg bg-primary/10 flex items-center justify-center text-primary font-bold text-xs">
                        <ReceiptText className="size-4" />
                      </div>
                      <h2 className="text-base font-bold tracking-tight text-foreground">
                        {t('billing.officialInvoiceTitle')}
                      </h2>
                    </div>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {t('billing.electronicDocDisclaimer')}
                    </p>
                  </div>

                  <div className="sm:text-right space-y-1">
                    <span className="inline-flex items-center gap-1 text-[10px] font-bold uppercase tracking-wider text-muted-foreground px-2 py-0.5 rounded-md bg-muted/40 border border-border/60">
                      {t('billing.invoiceNumber')}
                    </span>
                    <h3 className="text-base font-mono font-bold tracking-tight text-foreground pt-0.5">
                      {getInvoiceNumber(selectedInvoice)}
                    </h3>
                    <p className="text-xs text-muted-foreground">
                      {t('billing.servicePeriod')}: <span className="font-medium text-foreground">{formatDate(selectedInvoice.period_start)} – {formatDate(selectedInvoice.period_end)}</span>
                    </p>
                    {selectedInvoice.paid_at && (
                      <p className="text-xs text-emerald-600 dark:text-emerald-400 font-medium">
                        {t('billing.paidOn', { date: formatDate(selectedInvoice.paid_at) })}
                      </p>
                    )}
                  </div>
                </div>

                {/* Billed To & Payment Meta */}
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 p-4 rounded-xl border border-border/60 bg-muted/20">
                  <div>
                    <p className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                      {t('billing.billedTo')}:
                    </p>
                    <h4 className="mt-0.5 text-xs font-bold text-foreground">
                      {profile.company_name || profile.email || '—'}
                    </h4>
                    <div className="mt-1 space-y-0.5 text-[11px] text-muted-foreground">
                      {profile.tax_id && (
                        <p>
                          <span className="font-medium">{t('billing.profile.taxId')}:</span>{' '}
                          <span className="font-mono text-foreground">{profile.tax_id}</span>
                        </p>
                      )}
                      {profile.address_line1 && (
                        <p>
                          {profile.address_line1}, {profile.city} {profile.postal_code}
                        </p>
                      )}
                      {profile.email && <p>{profile.email}</p>}
                    </div>
                  </div>

                  <div className="sm:text-right space-y-2">
                    <div>
                      <p className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                        {t('billing.invoiceStatus')}
                      </p>
                      <div className="mt-1 flex sm:justify-end">
                        <Badge
                          variant={statusVariant(selectedInvoice.status)}
                          className="text-[10px] font-bold uppercase tracking-wider px-2 py-0.5"
                        >
                          {formatStatus(selectedInvoice.status)}
                        </Badge>
                      </div>
                    </div>
                    <div>
                      <p className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                        {t('billing.paymentMethod')}
                      </p>
                      <p className="text-xs font-medium text-foreground">{t('billing.paymentMethodWallet')}</p>
                    </div>
                  </div>
                </div>

                {/* Itemized Line Items Table */}
                <div className="space-y-2">
                  <h4 className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground">
                    {t('billing.itemDescription')}
                  </h4>
                  <div className="rounded-xl border border-border/60 overflow-hidden bg-card">
                    <Table>
                      <TableHeader className="bg-muted/40">
                        <TableRow className="border-b border-border/40">
                          <TableHead className="text-[10px] font-bold uppercase text-muted-foreground pl-4">
                            {t('billing.itemDescription')}
                          </TableHead>
                          <TableHead className="text-[10px] font-bold uppercase text-muted-foreground">
                            {t('billing.periodLabel')}
                          </TableHead>
                          <TableHead className="text-[10px] font-bold uppercase text-muted-foreground text-right">
                            {t('billing.credits')}
                          </TableHead>
                          <TableHead className="text-[10px] font-bold uppercase text-muted-foreground text-right pr-4">
                            {t('billing.amountIdr')}
                          </TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {overview.status === 'success' && overview.data.resources.length > 0 ? (
                          overview.data.resources.map((res) => (
                            <TableRow key={`${res.resource_type}-${res.resource_id}`} className="border-b border-border/30 last:border-0">
                              <TableCell className="pl-4 py-2.5">
                                <div className="flex items-center gap-2">
                                  {res.resource_type === 'project' ? (
                                    <FolderGit2 className="size-3.5 text-cyan-500 shrink-0" />
                                  ) : (
                                    <Database className="size-3.5 text-purple-500 shrink-0" />
                                  )}
                                  <div>
                                    <p className="text-xs font-semibold text-foreground">
                                      {res.resource_name || `${translateResourceType(res.resource_type)} #${res.resource_id}`}
                                    </p>
                                    <p className="text-[10px] text-muted-foreground">
                                      {res.spec_name}
                                    </p>
                                  </div>
                                </div>
                              </TableCell>
                              <TableCell className="text-[11px] text-muted-foreground font-mono">
                                1 {t('billing.month')}
                              </TableCell>
                              <TableCell className="text-xs font-bold text-foreground text-right font-mono tabular-nums">
                                {formatCredits(res.monthly_credits)} {t('billing.credits')}
                              </TableCell>
                              <TableCell className="text-xs font-semibold text-foreground text-right pr-4 font-mono tabular-nums">
                                Rp {(res.monthly_credits * 1000).toLocaleString(language === 'id' ? 'id-ID' : 'en-US')}
                              </TableCell>
                            </TableRow>
                          ))
                        ) : (
                          <TableRow className="border-b border-border/30 last:border-0">
                            <TableCell className="pl-4 py-3">
                              <div className="flex items-center gap-2">
                                <ReceiptText className="size-3.5 text-primary shrink-0" />
                                <div>
                                  <p className="text-xs font-semibold text-foreground">{t('billing.resourceBilling')}</p>
                                  <p className="text-[10px] text-muted-foreground">{t('billing.resourceBillingDescription')}</p>
                                </div>
                              </div>
                            </TableCell>
                            <TableCell className="text-[11px] text-muted-foreground font-mono">
                              {formatDate(selectedInvoice.period_start)} – {formatDate(selectedInvoice.period_end)}
                            </TableCell>
                            <TableCell className="text-xs font-bold text-foreground text-right font-mono tabular-nums">
                              {formatCredits(selectedInvoice.total_credits)} {t('billing.credits')}
                            </TableCell>
                            <TableCell className="text-xs font-semibold text-foreground text-right pr-4 font-mono tabular-nums">
                              Rp {(selectedInvoice.total_credits * 1000).toLocaleString(language === 'id' ? 'id-ID' : 'en-US')}
                            </TableCell>
                          </TableRow>
                        )}
                      </TableBody>
                    </Table>
                  </div>
                </div>

                {/* Subtotal & Total Box */}
                <div className="flex justify-end pt-1">
                  <div className="w-full sm:w-72 space-y-2 rounded-xl border border-border/60 bg-muted/20 p-3.5 text-xs">
                    <div className="flex justify-between text-muted-foreground">
                      <span>{t('billing.subtotal')}</span>
                      <span className="font-mono font-medium text-foreground tabular-nums">
                        {formatCredits(selectedInvoice.total_credits)} {t('billing.credits')}
                      </span>
                    </div>
                    <div className="flex justify-between text-muted-foreground">
                      <span>{t('billing.taxRate')}</span>
                      <span className="font-mono font-medium text-foreground">Rp 0</span>
                    </div>
                    <div className="border-t border-border/40 pt-2 flex justify-between items-baseline">
                      <span className="font-bold text-foreground uppercase tracking-wider text-[11px]">
                        {t('billing.totalCharged')}
                      </span>
                      <div className="text-right">
                        <div className="text-sm font-bold text-primary font-mono tabular-nums">
                          {formatCredits(selectedInvoice.total_credits)} {t('billing.credits')}
                        </div>
                        <div className="text-[11px] text-muted-foreground font-mono tabular-nums">
                          ≈ Rp {(selectedInvoice.total_credits * 1000).toLocaleString(language === 'id' ? 'id-ID' : 'en-US')}
                        </div>
                      </div>
                    </div>
                  </div>
                </div>

                {/* Legal Electronic Stamp Footer */}
                <div className="pt-3 border-t border-border/40 text-center text-[10px] text-muted-foreground leading-relaxed">
                  {t('billing.electronicDocDisclaimer')}
                </div>
              </div>
            </div>
          )}
        </DialogContent>
      </Dialog>

    </div>
  )
}
