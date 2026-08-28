import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ChangeEvent,
  type Dispatch,
  type FormEvent,
  type SetStateAction,
} from 'react'
import { RefreshCw } from 'lucide-react'
import { toast } from 'sonner'
import axios from 'axios'

import { BillingAlerts } from '@/components/billing/BillingAlerts'
import { BillingCreditCatalog } from '@/components/billing/BillingCreditCatalog'
import {
  InvoiceDialog,
  PaymentDialog,
  ProfileRequiredDialog,
  RenewConfirmationDialog,
  TopupConfirmationDialog,
} from '@/components/billing/BillingDialogs'
import { BillingHistorySection } from '@/components/billing/BillingHistorySection'
import { BillingProfileSection } from '@/components/billing/BillingProfileSection'
import { ResourceBillingCard } from '@/components/billing/ResourceBillingCard'
import type { PendingRenewChange, PendingTopup } from '@/components/billing/types'
import { isBillingProfileComplete } from '@/components/billing/utils'
import { useBillingFormatters } from '@/components/billing/useBillingFormatters'
import { Button } from '@/components/ui/button'
import {
  createTopupIdempotencyKey,
  hasLowCreditBalance,
  nextBillingRequestState,
  type BillingRequestState,
} from '@/lib/billing-ui'
import { scrollIntoMain } from '@/lib/scrollIntoMain'
import { usePolling } from '@/lib/usePolling'
import { billingAPI } from '@/services/api'
import type { BillingOverview, BillingStatus, TopupPackage, BillingProfile, TopupResponse } from '@/types'

export default function Billing() {
  const [profile, setProfile] = useState<BillingProfile>({
    company_name: '', tax_id: '', email: '', phone: '',
    address_line1: '', address_line2: '', city: '', state_province: '',
    postal_code: '', country: 'ID'
  })
  const isProfileDirtyRef = useRef(false)
  const handleUpdateProfile = useCallback((updater: SetStateAction<BillingProfile>) => {
    isProfileDirtyRef.current = true
    setProfile(updater)
  }, [])
  const [savingProfile, setSavingProfile] = useState(false)
  const [isProfileSaved, setIsProfileSaved] = useState(false)
  const [activePaymentModal, setActivePaymentModal] = useState<TopupResponse | null>(null)
  const [checkingPaymentStatus, setCheckingPaymentStatus] = useState(false)

  const { t, language, formatMoney } = useBillingFormatters()
  const [overview, setOverview] = useState<BillingRequestState<BillingOverview>>({ status: 'idle' })
  const [packages, setPackages] = useState<BillingRequestState<TopupPackage[]>>({ status: 'idle' })
  const [statuses, setStatuses] = useState<BillingRequestState<BillingStatus[]>>({ status: 'idle' })
  const [renewLoading, setRenewLoading] = useState<Record<string, boolean>>({})
  const [paymentLoading, setPaymentLoading] = useState<Record<string, boolean>>({})
  const [pendingRenewChange, setPendingRenewChange] = useState<PendingRenewChange | null>(null)
  const [topupPackageID, setTopupPackageID] = useState<number | null>(null)
  const [pendingTopup, setPendingTopup] = useState<PendingTopup | null>(null)
  const [customAmount, setCustomAmount] = useState('')
  const [staleWarning, setStaleWarning] = useState(false)
  const [showProfilePrompt, setShowProfilePrompt] = useState(false)
  const topupKeys = useRef<Map<number, string>>(new Map())
  const customTopupKey = useRef<string | null>(null)
  const loadInFlight = useRef(false)
  const hasLoadedProfileRef = useRef(false)

  const [selectedInvoice, setSelectedInvoice] = useState<BillingOverview['invoices'][0] | null>(null)
  const [copiedInvoiceID, setCopiedInvoiceID] = useState<number | null>(null)

  const handleCopyInvoiceNumber = (invoiceNumber: string, id: number) => {
    void navigator.clipboard.writeText(invoiceNumber)
    setCopiedInvoiceID(id)
    toast.success(t('billing.copiedInvoice'))
    setTimeout(() => setCopiedInvoiceID(null), 2000)
  }

  const scrollToBillingProfile = () => scrollIntoMain('billing-profile-card')

  const checkProfileAndPrompt = (): boolean => {
    if (!isProfileSaved && !isBillingProfileComplete(profile)) {
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

  const didLoadInitial = useRef(false)

  const load = useCallback(async () => {
    if (loadInFlight.current) return
    loadInFlight.current = true

    const markLoading = <T,>(setter: Dispatch<SetStateAction<BillingRequestState<T>>>) => {
      setter((current) => (current.status === 'success' ? current : { status: 'loading' }))
    }

    markLoading(setOverview)
    markLoading(setPackages)
    markLoading(setStatuses)

    const currentOverview = overviewRef.current
    const currentPackages = packagesRef.current
    const currentStatuses = statusesRef.current

    const [overviewResult, catalogResult, statusResult] = await Promise.allSettled([
      billingAPI.overview(),
      billingAPI.catalog(),
      billingAPI.status(),
    ])

    const nextOverview = nextBillingRequestState(overviewResult, currentOverview, (response) => response.data)
    const nextPackages = nextBillingRequestState(catalogResult, currentPackages, (response) => response.data.packages)
    const nextStatuses = nextBillingRequestState(statusResult, currentStatuses, (response) => response.data)
    const hasStaleData =
      (overviewResult.status === 'rejected' && nextOverview.status === 'success') ||
      (catalogResult.status === 'rejected' && nextPackages.status === 'success') ||
      (statusResult.status === 'rejected' && nextStatuses.status === 'success')

    let profileToSet: BillingProfile | null = null
    let markProfileSaved = false

    if (!hasLoadedProfileRef.current) {
      try {
        const resProf = await billingAPI.getProfile()
        // Mark loaded only after a conclusive response (success or authoritative not-found).
        // Network errors / 5xx leave the flag unset so the next poll can retry.
        hasLoadedProfileRef.current = true
        if (resProf.data) {
          const loadedProf: BillingProfile = {
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
          // Only hydrate profile if the user hasn't modified any form input (dirty protection)
          if (!isProfileDirtyRef.current) {
            profileToSet = loadedProf
          }
          if (isBillingProfileComplete(loadedProf)) {
            markProfileSaved = true
          }
        }
      } catch (e) {
        // Transient error (network, 5xx): leave hasLoadedProfileRef.current = false
        // so the next poll attempt will retry.
      }
    }

    // Reset in-flight ref BEFORE batching state updates so the scheduled re-render
    // evaluates loadInFlight.current === false (e.g. Refresh button is enabled).
    loadInFlight.current = false
    setOverview(nextOverview)
    setPackages(nextPackages)
    setStatuses(nextStatuses)
    setStaleWarning(hasStaleData)
    if (profileToSet) {
      setProfile(profileToSet)
    }
    if (markProfileSaved) {
      setIsProfileSaved(true)
    }
    // Reads current state via refs, so this stays referentially stable and the
    // polling effect no longer tears down and restarts on every render.
  }, [])

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
          toast.success(t('billing.paymentSuccess'))
        } else {
          toast.info(t('billing.paymentPending'))
        }
      } catch {
        toast.error(t('billing.paymentVerifyFailed'))
      } finally {
        await load()
      }
    })()
  }, [load])

  usePolling(() => void load(), 30_000)

  const attentionResources = useMemo(() => {
    if (statuses.status !== 'success') return []
    return statuses.data
      .filter(({ status }) => status === 'payment_due' || status === 'suspended')
      .map((statusItem) => {
        const matchingResource =
          overview.status === 'success'
            ? overview.data.resources.find(
                (r) => r.resource_type === statusItem.resource_type && r.resource_id === statusItem.resource_id,
              )
            : undefined
        return {
          ...statusItem,
          resource_name: matchingResource?.resource_name,
        }
      })
  }, [statuses, overview])

  const startTopup = async (packageID: number) => {
    if (!checkProfileAndPrompt()) return
    setTopupPackageID(packageID)
    try {
      const idempotencyKey = topupKeys.current.get(packageID) ?? createTopupIdempotencyKey()
      topupKeys.current.set(packageID, idempotencyKey)
      const response = await billingAPI.createTopup(packageID, idempotencyKey)
      if (response.data.payment_token && response.data.payment_token.startsWith('000201')) {
        topupKeys.current.delete(packageID)
        setPendingTopup(null)
        setActivePaymentModal(response.data)
        return
      }
      if (!response.data.payment_url) {
        topupKeys.current.delete(packageID)
        throw new Error(t('billing.paymentSessionUnavailable'))
      }
      topupKeys.current.delete(packageID)
      setPendingTopup(null)
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

  const baseRatePerCredit = useMemo(() => {
    if (packages.status !== 'success' || packages.data.length === 0) return 1000
    const activeIdrPackages = packages.data
      .filter((p) => (p.currency === 'IDR' || !p.currency) && p.credits > 0 && p.amount_minor > 0)
      .sort((a, b) => a.credits - b.credits)
    if (activeIdrPackages.length === 0) return 1000
    const base = activeIdrPackages[0]
    const rate = Math.floor(base.amount_minor / base.credits)
    return rate > 0 ? rate : 1000
  }, [packages])

  const customAmountNum = parseInt(customAmount.replace(/\D/g, ''), 10) || 0
  const customCredits = Math.floor(customAmountNum / baseRatePerCredit)
  const customValid =
    customAmountNum >= 10_000 &&
    customAmountNum <= 10_000_000 &&
    customAmountNum % 1000 === 0

  const handleCustomAmountChange = (e: ChangeEvent<HTMLInputElement>) => {
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

  const startCustomTopup = async (amountToUse?: number) => {
    if (!checkProfileAndPrompt()) return
    const amount = amountToUse ?? customAmountNum
    if (amount <= 0) return
    setTopupPackageID(-1)
    try {
      const idempotencyKey = customTopupKey.current ?? createTopupIdempotencyKey()
      customTopupKey.current = idempotencyKey
      const response = await billingAPI.createTopup(0, idempotencyKey, amount)
      if (response.data.payment_token && response.data.payment_token.startsWith('000201')) {
        customTopupKey.current = null
        setPendingTopup(null)
        setActivePaymentModal(response.data)
        return
      }
      if (!response.data.payment_url) {
        customTopupKey.current = null
        throw new Error(t('billing.paymentSessionUnavailable'))
      }
      customTopupKey.current = null
      setCustomAmount('')
      setPendingTopup(null)
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

  const handleInitiatePackageTopup = (pkg: TopupPackage) => {
    if (!checkProfileAndPrompt()) return
    setPendingTopup({
      type: 'package',
      package: pkg,
      credits: pkg.credits,
      amountMinor: pkg.amount_minor,
      currency: pkg.currency,
    })
  }

  const handleInitiateCustomTopup = () => {
    if (!checkProfileAndPrompt()) return
    if (!customValid || customAmountNum <= 0) return
    setPendingTopup({
      type: 'custom',
      customAmount: customAmountNum,
      credits: customCredits,
      amountMinor: customAmountNum,
      currency: 'IDR',
    })
  }

  const handleConfirmTopup = async () => {
    if (!pendingTopup) return
    if (pendingTopup.type === 'package' && pendingTopup.package) {
      await startTopup(pendingTopup.package.id)
    } else if (pendingTopup.type === 'custom' && pendingTopup.customAmount) {
      await startCustomTopup(pendingTopup.customAmount)
    }
  }

  const conversionRateFormatted = useMemo(() => {
    if (!pendingTopup) return ''
    const unitAmount =
      pendingTopup.type === 'package' && pendingTopup.package && pendingTopup.credits > 0
        ? Math.floor(pendingTopup.amountMinor / pendingTopup.credits)
        : baseRatePerCredit
    return formatMoney(unitAmount, pendingTopup.currency)
  }, [pendingTopup, baseRatePerCredit, formatMoney])

  const reconcileTopup = async (topupID: number) => {
    try {
      await billingAPI.reconcileTopup(topupID)
      await load()
      toast.success(t('billing.statusRefreshed'))
    } catch {
      toast.error(t('billing.statusRefreshFailed'))
    }
  }

  const confirmRenewChange = async () => {
    if (!pendingRenewChange) return
    const { resource_id, resource_type, target_auto_renew } = pendingRenewChange
    const key = `${resource_type}-${resource_id}`
    setPendingRenewChange(null)
    setRenewLoading((current) => ({ ...current, [key]: true }))
    try {
      await billingAPI.updateAutoRenew(resource_id, resource_type, target_auto_renew)
      await load()
      toast.success(target_auto_renew ? t('billing.autoRenewEnabled') : t('billing.autoRenewDisabled'))
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
      setRenewLoading((current) => ({ ...current, [key]: false }))
    }
  }

  const payDueResource = async (resourceID: number, resourceType: 'project' | 'database') => {
    const key = `${resourceType}-${resourceID}`
    setPaymentLoading((current) => ({ ...current, [key]: true }))
    try {
      await billingAPI.payDueResource(resourceID, resourceType)
      await load()
      toast.success(t('billing.overduePaymentSuccess'))
    } catch (error) {
      if (axios.isAxiosError(error) && error.response?.data?.code === 'INSUFFICIENT_CREDITS') {
        toast.error(t('billing.overdueInsufficientCredits'))
      } else {
        toast.error(t('billing.overduePaymentFailed'))
      }
    } finally {
      setPaymentLoading((current) => ({ ...current, [key]: false }))
    }
  }

  const balanceData = overview.status === 'success' ? overview.data.wallet.balance_credits : null
  const upcomingCredits = overview.status === 'success' ? overview.data.upcoming_required_credits : null
  const showLowBalance =
    balanceData !== null &&
    upcomingCredits !== null &&
    hasLowCreditBalance(balanceData, upcomingCredits) &&
    attentionResources.length === 0


  const handleSaveProfile = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    setSavingProfile(true)
    try {
      const res = await billingAPI.updateProfile(profile)
      isProfileDirtyRef.current = false
      hasLoadedProfileRef.current = true
      setIsProfileSaved(true)
      if (res.data) {
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

  // Track active top-up ID via ref to isolate concurrent/stale async responses.
  const activePaymentModalIdRef = useRef<number | null>(null)
  useEffect(() => {
    activePaymentModalIdRef.current = activePaymentModal?.id ?? null
  }, [activePaymentModal])

  // In-flight guard keyed by top-up ID.
  const inFlightTopupIdRef = useRef<number | null>(null)

  const handleCheckStatusModal = async (isManual = false) => {
    const currentModal = activePaymentModal
    if (!currentModal) return
    const topupId = currentModal.id

    if (inFlightTopupIdRef.current === topupId) return
    inFlightTopupIdRef.current = topupId
    if (isManual) setCheckingPaymentStatus(true)

    try {
      const response = await billingAPI.reconcileTopup(topupId)

      // Guard against stale response: if user closed or switched to a different top-up, ignore
      if (activePaymentModalIdRef.current !== topupId) {
        return
      }

      const status = response.data.status
      if (status === 'pending') {
        if (isManual) {
          toast.info(t('billing.paymentPending'))
        }
        return
      }

      // Terminal status received: refresh dashboard overview/status data
      await load()

      // Re-verify modal is still active for this top-up ID before dismissing
      if (activePaymentModalIdRef.current !== topupId) {
        return
      }

      if (status === 'paid') {
        toast.success(t('billing.paymentSuccess'))
        setActivePaymentModal(null)
      } else if (status === 'failed' || status === 'expired') {
        toast.error(t('billing.paymentFailed'))
        setActivePaymentModal(null)
      } else {
        // Other terminal states: refunded, partial_refund, chargeback, void
        toast.info(t('billing.paymentEnded'))
        setActivePaymentModal(null)
      }
    } catch {
      if (activePaymentModalIdRef.current === topupId && isManual) {
        toast.error(t('billing.paymentVerifyFailed'))
      }
    } finally {
      if (inFlightTopupIdRef.current === topupId) {
        inFlightTopupIdRef.current = null
      }
      if (isManual) {
        setCheckingPaymentStatus(false)
      }
    }
  }

  // 5-second polling scoped to the currently active payment modal.
  // Re-creates timer when activePaymentModal changes and cleans up when closed/unmounted.
  useEffect(() => {
    if (!activePaymentModal) return
    const currentId = activePaymentModal.id
    const interval = setInterval(() => {
      if (activePaymentModalIdRef.current === currentId) {
        void handleCheckStatusModal(false)
      }
    }, 5_000)
    return () => clearInterval(interval)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activePaymentModal?.id])

  return (
    <div className="mx-auto max-w-6xl space-y-8 pb-12 animate-in fade-in duration-500">
      <div className="flex flex-col gap-4 border-b border-border/60 pb-6 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <div className="flex items-center gap-1.5 text-xs text-muted-foreground font-medium mb-1">
            <span>{t('common.dashboard')}</span>
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

      <BillingAlerts
        staleWarning={staleWarning}
        attentionResources={attentionResources}
        showLowBalance={showLowBalance}
      />
      <BillingCreditCatalog
        overview={overview}
        packages={packages}
        topupPackageID={topupPackageID}
        customAmount={customAmount}
        customAmountNum={customAmountNum}
        customCredits={customCredits}
        customValid={customValid}
        handleInitiatePackageTopup={handleInitiatePackageTopup}
        handleCustomAmountChange={handleCustomAmountChange}
        handleInitiateCustomTopup={handleInitiateCustomTopup}
      />
      <ResourceBillingCard
        overview={overview}
        statuses={statuses}
        renewLoading={renewLoading}
        paymentLoading={paymentLoading}
        payDueResource={payDueResource}
        setPendingRenewChange={setPendingRenewChange}
      />
      <RenewConfirmationDialog
        pendingRenewChange={pendingRenewChange}
        setPendingRenewChange={setPendingRenewChange}
        confirmRenewChange={confirmRenewChange}
      />
      <BillingHistorySection
        overview={overview}
        copiedInvoiceID={copiedInvoiceID}
        handleCopyInvoiceNumber={handleCopyInvoiceNumber}
        setSelectedInvoice={setSelectedInvoice}
        reconcileTopup={reconcileTopup}
        onPayPendingTopup={(topup) => {
          setActivePaymentModal({
            id: topup.id,
            credits: topup.credits,
            amount_minor: topup.amount_minor,
            currency: topup.currency,
            status: topup.status,
            payment_token: topup.payment_token,
            payment_url: topup.payment_url,
          })
        }}
      />
      <BillingProfileSection
        profile={profile}
        setProfile={handleUpdateProfile}
        savingProfile={savingProfile}
        isProfileSaved={isProfileSaved}
        handleSaveProfile={handleSaveProfile}
      />
      <ProfileRequiredDialog
        showProfilePrompt={showProfilePrompt}
        setShowProfilePrompt={setShowProfilePrompt}
        scrollToBillingProfile={scrollToBillingProfile}
      />
      <TopupConfirmationDialog
        pendingTopup={pendingTopup}
        setPendingTopup={setPendingTopup}
        topupPackageID={topupPackageID}
        overview={overview}
        conversionRateFormatted={conversionRateFormatted}
        handleConfirmTopup={handleConfirmTopup}
      />
      <PaymentDialog
        activePaymentModal={activePaymentModal}
        setActivePaymentModal={setActivePaymentModal}
        checkingPaymentStatus={checkingPaymentStatus}
        handleCheckStatusModal={() => handleCheckStatusModal(true)}
      />
      <InvoiceDialog
        selectedInvoice={selectedInvoice}
        setSelectedInvoice={setSelectedInvoice}
        profile={profile}
        copiedInvoiceID={copiedInvoiceID}
        handleCopyInvoiceNumber={handleCopyInvoiceNumber}
      />
    </div>
  )
}
