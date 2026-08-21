import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  AlertTriangle,
  Boxes,
  Calendar,
  ChevronRight,
  Coins,
  CreditCard,
  Filter,
  Layers,
  Pencil,
  Plus,
  ReceiptText,
  RefreshCw,
  Search,
  ShieldAlert,
  WalletCards,
} from 'lucide-react'
import { toast } from 'sonner'
import { billingAPI, usersAPI } from '@/services/api'
import { User } from '@/types'
import { createAdjustmentIdempotencyKey, SUPPORTED_CURRENCIES, toMajorUnits } from '@/lib/billing-ui'
import useAuthStore from '@/stores/authStore'
import useTranslation from '@/lib/useTranslation'
import { scrollIntoMain } from '@/lib/scrollIntoMain'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { TablePagination } from '@/components/ui/table-pagination'
import { serverPagination, usePagination } from '@/lib/pagination'

type Catalog = {
  specs: Array<{
    id: number
    name: string
    slug: string
    type: 'project' | 'database'
    version: number
    monthly_credits: number
    cpu_millicores: number
    memory_mb: number
    storage_gb: number
    connection_limit?: number
    backup_retention_days?: number
    badge_text?: string
    is_active: boolean
  }>
  packages: Array<{
    id: number
    credits: number
    amount_minor: number
    currency: string
    sort_order: number
    version: number
    is_active: boolean
  }>
}
type Suspension = { user_id: number; resource_id: number; resource_type: string; status: string; oldest_due_at?: string; payment_due_days: number }
type Page<T> = { data: T[]; page: number; limit: number; total: number }
type Wallet = { user_id: number; balance_credits: number; updated_at: string }
type InvoiceItem = { id: number; resource_type: string; resource_name: string; spec_name: string; description: string; credits: number }
type Invoice = { id: number; user_id: number; total_credits: number; status: string; period_start: string; period_end: string; due_at?: string; paid_at?: string; created_at?: string; items?: InvoiceItem[] }
type Topup = { id: number; user_id: number; credits: number; amount_minor: number; currency: string; status: string; created_at: string; paid_at?: string }
type TopupPackage = { id: number; credits: number; amount_minor: number; currency: string; sort_order: number }
type SpecForm = {
  type: 'project' | 'database'
  name: string
  slug: string
  cpu_millicores: number | ''
  memory_mb: number | ''
  storage_gb: number | ''
  monthly_credits: number | ''
  connection_limit: number | ''
  backup_retention_days: number | ''
  badge_text: string
  reason: string
}

const PAGE_LIMIT = 10
const emptySpec: SpecForm = { type: 'project', name: '', slug: '', cpu_millicores: '', memory_mb: '', storage_gb: '', monthly_credits: '', connection_limit: '', backup_retention_days: '', badge_text: '', reason: '' }
const slugify = (text: string) => text.toLowerCase().trim().replace(/[^a-z0-9]+/g, '-').replace(/(^-|-$)+/g, '')

interface PackageFormState {
  credits: number | ''
  currency: string
  amount_minor: number | ''
  sort_order: number | ''
  reason: string
}

interface PackageEditFormState {
  credits: number | ''
  amount_minor: number | ''
  sort_order: number | ''
  reason: string
}

interface CreditAdjustmentFormState {
  credits: number | ''
  reason: string
}

interface DirectCreditAdjustmentFormState {
  user_id: number | ''
  credits: number | ''
  reason: string
}

const emptyPackage: PackageFormState = { credits: '', currency: 'IDR', amount_minor: '', sort_order: '', reason: '' }
const emptyPackageEdit: PackageEditFormState = { credits: '', amount_minor: '', sort_order: '', reason: '' }
const emptyCreditAdjustment: CreditAdjustmentFormState = { credits: '', reason: '' }
const emptyDirectCreditAdjustment: DirectCreditAdjustmentFormState = { user_id: '', credits: '', reason: '' }

const statusVariant = (status: string): "default" | "secondary" | "destructive" | "outline" => {
  switch (status) {
    case 'paid':
    case 'active':
      return 'secondary'
    case 'suspended':
    case 'payment_due':
    case 'failed':
      return 'destructive'
    case 'pending':
      return 'outline'
    default:
      return 'outline'
  }
}

export default function AdminBilling() {
  const { t, language } = useTranslation()
  const isSuperAdmin = useAuthStore((state) => state.user?.role === 'superadmin')
  const [catalog, setCatalog] = useState<Catalog | null>(null)
  const [wallets, setWallets] = useState<Page<Wallet> | null>(null)
  const [invoices, setInvoices] = useState<Page<Invoice> | null>(null)
  const [topups, setTopups] = useState<Page<Topup> | null>(null)
  const [suspensions, setSuspensions] = useState<Suspension[] | null>(null)
  const [usersList, setUsersList] = useState<User[]>([])
  
  const [walletPage, setWalletPage] = useState(1)
  const [invoicePage, setInvoicePage] = useState(1)
  const [topupPage, setTopupPage] = useState(1)
  const [walletLimit, setWalletLimit] = useState(PAGE_LIMIT)
  const [invoiceLimit, setInvoiceLimit] = useState(PAGE_LIMIT)
  const [topupLimit, setTopupLimit] = useState(PAGE_LIMIT)
  
  // Period filter state
  const [selectedPeriod, setSelectedPeriod] = useState<string>('this_month')
  
  // Search & Filter state
  const [walletSearch, setWalletSearch] = useState('')
  const [invoiceSearch, setInvoiceSearch] = useState('')
  const [invoiceStatusFilter, setInvoiceStatusFilter] = useState<string>('all')
  const [topupSearch, setTopupSearch] = useState('')
  const [topupStatusFilter, setTopupStatusFilter] = useState<string>('all')
  
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [loading, setLoading] = useState(true)
  
  const [specForm, setSpecForm] = useState(emptySpec)
  const [editingSpec, setEditingSpec] = useState<Catalog['specs'][number] | null>(null)
  const [packageForm, setPackageForm] = useState(emptyPackage)
  const [editingPackage, setEditingPackage] = useState<TopupPackage | null>(null)
  const [packageEditForm, setPackageEditForm] = useState(emptyPackageEdit)
  const [adjustingWallet, setAdjustingWallet] = useState<Wallet | null>(null)
  const [creditAdjustmentForm, setCreditAdjustmentForm] = useState(emptyCreditAdjustment)
  const [creatingSpec, setCreatingSpec] = useState(false)
  const [creatingPackage, setCreatingPackage] = useState(false)
  const [savingPackageEdit, setSavingPackageEdit] = useState(false)
  const [savingCreditAdjustment, setSavingCreditAdjustment] = useState(false)
  const [savingDirectCreditAdjustment, setSavingDirectCreditAdjustment] = useState(false)
  const [directCreditAdjustmentForm, setDirectCreditAdjustmentForm] = useState(emptyDirectCreditAdjustment)
  const [isDirectCreditModalOpen, setIsDirectCreditModalOpen] = useState(false)
  const [viewingInvoice, setViewingInvoice] = useState<Invoice | null>(null)

  // Catalog filters
  const [specSearch, setSpecSearch] = useState('')
  const [specTypeFilter, setSpecTypeFilter] = useState<string>('all')
  const [specActiveFilter, setSpecActiveFilter] = useState<string>('active')
  const [packageSearch, setPackageSearch] = useState('')
  const [packageActiveFilter, setPackageActiveFilter] = useState<string>('active')

  const locale = language === 'id' ? 'id-ID' : 'en-US'
  // Intl constructors are expensive; one per table cell dominated render cost.
  // Build once per locale instead.
  const numberFormat = useMemo(() => new Intl.NumberFormat(locale), [locale])
  const dateFormat = useMemo(() => new Intl.DateTimeFormat(locale, { dateStyle: 'medium' }), [locale])
  const formatCredits = useCallback((value: number) => numberFormat.format(value), [numberFormat])
  const formatDate = useCallback((value?: string) => (value ? dateFormat.format(new Date(value)) : '—'), [dateFormat])
  // ponytail: currency varies per row, so cache one formatter per currency seen.
  // Cache is rebuilt with the closure whenever locale changes.
  const formatMoney = useMemo(() => {
    const cache = new Map<string, Intl.NumberFormat>()
    return (amountMinor: number, currency: string) => {
      let fmt = cache.get(currency)
      if (!fmt) {
        fmt = new Intl.NumberFormat(locale, { style: 'currency', currency, maximumFractionDigits: 0 })
        cache.set(currency, fmt)
      }
      return fmt.format(toMajorUnits(amountMinor, currency))
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
    [t]
  )

  const usersMap = useMemo(() => {
    const map = new Map<number, User>()
    usersList.forEach((u) => map.set(u.id, u))
    return map
  }, [usersList])

  const getUserDetails = useCallback((userId: number) => {
    const user = usersMap.get(userId)
    if (user) {
      const initials = (user.name || user.email || 'U').split(' ').map((n) => n[0]).join('').substring(0, 2).toUpperCase()
      return { name: user.name, email: user.email, role: user.role, initials, found: true }
    }
    return { name: `User #${userId}`, email: '', role: '', initials: `U${userId}`, found: false }
  }, [usersMap])

  const load = useCallback(async () => {
    setLoading(true)
    try {
      if (isSuperAdmin) {
        usersAPI.list({ limit: 100 }).then((res) => {
          const list = res.data?.data || res.data || []
          setUsersList(Array.isArray(list) ? list : [])
        }).catch(() => {})
      }
      const results = await Promise.allSettled([
        billingAPI.adminCatalog(),
        billingAPI.adminWallets({ page: walletPage, limit: walletLimit }),
        billingAPI.adminInvoices({ page: invoicePage, limit: invoiceLimit }),
        billingAPI.adminTopups({ page: topupPage, limit: topupLimit }),
        billingAPI.adminSuspensions(),
      ])
      const nextErrors: Record<string, string> = {}
      if (results[0].status === 'fulfilled') setCatalog(results[0].value.data); else nextErrors.plans = t('billing.plans')
      if (results[1].status === 'fulfilled') setWallets(results[1].value.data); else nextErrors.wallets = t('billing.admin.wallets')
      if (results[2].status === 'fulfilled') setInvoices(results[2].value.data); else nextErrors.invoices = t('billing.invoices')
      if (results[3].status === 'fulfilled') setTopups(results[3].value.data); else nextErrors.topups = t('billing.topups')
      if (results[4].status === 'fulfilled') setSuspensions(results[4].value.data); else nextErrors.suspensions = t('billing.admin.suspensions')
      setErrors(nextErrors)
    } finally {
      setLoading(false)
    }
  }, [invoicePage, t, topupPage, walletPage, walletLimit, invoiceLimit, topupLimit, isSuperAdmin])

  // .sort() mutates in place — sorting catalog.specs directly in JSX reordered
  // the state array on every render. Copy first.
  const sortedSpecs = useMemo(() => {
    const query = specSearch.toLowerCase().trim()
    return [...(catalog?.specs ?? [])]
      .filter((spec) => {
        if (specTypeFilter !== 'all' && spec.type !== specTypeFilter) return false
        if (specActiveFilter === 'active' && !spec.is_active) return false
        if (specActiveFilter === 'inactive' && spec.is_active) return false
        if (!query) return true
        return spec.name.toLowerCase().includes(query) || spec.slug.toLowerCase().includes(query)
      })
      .sort((a, b) => (a.is_active === b.is_active ? b.version - a.version : a.is_active ? -1 : 1))
  }, [catalog, specSearch, specTypeFilter, specActiveFilter])

  const sortedPackages = useMemo(() => {
    const query = packageSearch.toLowerCase().trim()
    return [...(catalog?.packages ?? [])]
      .filter((pkg) => {
        if (packageActiveFilter === 'active' && !pkg.is_active) return false
        if (packageActiveFilter === 'inactive' && pkg.is_active) return false
        if (!query) return true
        return String(pkg.credits).includes(query) || String(toMajorUnits(pkg.amount_minor, pkg.currency)).includes(query)
      })
      .sort((a, b) => (a.is_active === b.is_active ? a.sort_order - b.sort_order : a.is_active ? -1 : 1))
  }, [catalog, packageSearch, packageActiveFilter])
  const specPaging = usePagination(sortedSpecs.length)
  const packagePaging = usePagination(sortedPackages.length)

  useEffect(() => {
    void load()
  }, [load])

  const getErrorMessage = (err: unknown, fallback: string) => {
    return (err as { response?: { data?: { message?: string } } })?.response?.data?.message || fallback
  }

  const editSpec = (id: number) => {
    const spec = catalog?.specs.find((item) => item.id === id)
    if (!spec) return
    setEditingSpec(spec)
    setSpecForm({
      type: spec.type,
      name: spec.name,
      slug: spec.slug,
      cpu_millicores: spec.cpu_millicores,
      memory_mb: spec.memory_mb,
      storage_gb: spec.storage_gb,
      monthly_credits: spec.monthly_credits,
      connection_limit: spec.connection_limit ?? '',
      backup_retention_days: spec.backup_retention_days ?? '',
      badge_text: spec.badge_text ?? '',
      reason: '',
    })
    scrollIntoMain('spec-monthly-credits')
  }

  const createSpec = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!isSuperAdmin) return
    setCreatingSpec(true)
    try {
      const { connection_limit, backup_retention_days, ...spec } = specForm
      await billingAPI.createSpec(
        specForm.type === 'database'
          ? {
              ...spec,
              monthly_credits: Number(spec.monthly_credits || 0),
              cpu_millicores: Number(spec.cpu_millicores || 500),
              memory_mb: Number(spec.memory_mb || 512),
              storage_gb: Number(spec.storage_gb || 10),
              connection_limit: connection_limit === '' ? undefined : Number(connection_limit),
              backup_retention_days: backup_retention_days === '' ? undefined : Number(backup_retention_days),
            }
          : {
              ...spec,
              monthly_credits: Number(spec.monthly_credits || 0),
              cpu_millicores: Number(spec.cpu_millicores || 500),
              memory_mb: Number(spec.memory_mb || 512),
              storage_gb: Number(spec.storage_gb || 10),
            },
      )
      setSpecForm(emptySpec)
      setEditingSpec(null)
      toast.success(t('billing.admin.createSucceeded'))
      await load()
    } catch (err) {
      toast.error(getErrorMessage(err, t('billing.admin.createFailed')))
    } finally {
      setCreatingSpec(false)
    }
  }

  const createPackage = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!isSuperAdmin) return
    setCreatingPackage(true)
    try {
      await billingAPI.createTopupPackage({
        ...packageForm,
        credits: Number(packageForm.credits || 0),
        amount_minor: Number(packageForm.amount_minor || 0),
        sort_order: Number(packageForm.sort_order || 0),
      })
      setPackageForm(emptyPackage)
      toast.success(t('billing.admin.createSucceeded'))
      await load()
    } catch (err) {
      toast.error(getErrorMessage(err, t('billing.admin.createFailed')))
    } finally {
      setCreatingPackage(false)
    }
  }

  const startEditPackage = (pkg: TopupPackage) => {
    setEditingPackage(pkg)
    setPackageEditForm({ credits: pkg.credits, amount_minor: pkg.amount_minor, sort_order: pkg.sort_order, reason: '' })
  }

  const savePackageEdit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!isSuperAdmin || !editingPackage) return
    setSavingPackageEdit(true)
    try {
      await billingAPI.updateTopupPackage(editingPackage.id, {
        ...packageEditForm,
        credits: Number(packageEditForm.credits || 0),
        amount_minor: Number(packageEditForm.amount_minor || 0),
        sort_order: Number(packageEditForm.sort_order || 0),
      })
      setEditingPackage(null)
      setPackageEditForm(emptyPackageEdit)
      toast.success(t('billing.admin.updatePackageSuccess'))
      await load()
    } catch (err) {
      toast.error(getErrorMessage(err, t('billing.admin.updatePackageFailed')))
    } finally {
      setSavingPackageEdit(false)
    }
  }

  const walletAdjustmentKeyRef = useRef<string | null>(null)
  const directAdjustmentKeyRef = useRef<string | null>(null)

  const startAdjustWallet = (wallet: Wallet) => {
    walletAdjustmentKeyRef.current = null
    setAdjustingWallet(wallet)
    setCreditAdjustmentForm(emptyCreditAdjustment)
  }

  const saveCreditAdjustment = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!isSuperAdmin || !adjustingWallet) return
    setSavingCreditAdjustment(true)
    const idempotencyKey = walletAdjustmentKeyRef.current ?? createAdjustmentIdempotencyKey()
    walletAdjustmentKeyRef.current = idempotencyKey
    try {
      await billingAPI.adjustWalletCredits(
        adjustingWallet.user_id,
        { ...creditAdjustmentForm, credits: Number(creditAdjustmentForm.credits || 0) },
        idempotencyKey
      )
      walletAdjustmentKeyRef.current = null
      setAdjustingWallet(null)
      setCreditAdjustmentForm(emptyCreditAdjustment)
      toast.success(t('billing.admin.adjustmentSuccess'))
      await load()
    } catch (err) {
      toast.error(getErrorMessage(err, t('billing.admin.adjustmentFailed')))
    } finally {
      setSavingCreditAdjustment(false)
    }
  }

  const saveDirectCreditAdjustment = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!isSuperAdmin) return
    setSavingDirectCreditAdjustment(true)
    const idempotencyKey = directAdjustmentKeyRef.current ?? createAdjustmentIdempotencyKey()
    directAdjustmentKeyRef.current = idempotencyKey
    try {
      const { user_id, credits, reason } = directCreditAdjustmentForm
      await billingAPI.adjustWalletCredits(
        Number(user_id || 0),
        { credits: Number(credits || 0), reason },
        idempotencyKey
      )
      directAdjustmentKeyRef.current = null
      setDirectCreditAdjustmentForm(emptyDirectCreditAdjustment)
      setIsDirectCreditModalOpen(false)
      toast.success(t('billing.admin.adjustmentSuccess'))
      await load()
    } catch (err) {
      toast.error(getErrorMessage(err, t('billing.admin.adjustmentFailed')))
    } finally {
      setSavingDirectCreditAdjustment(false)
    }
  }

  // Statistics calculation
  const totalCirculatingCredits = useMemo(() => {
    return wallets?.data.reduce((acc, curr) => acc + (curr.balance_credits || 0), 0) ?? 0
  }, [wallets])

  const totalTopupsRevenue = useMemo(() => {
    return topups?.data.filter((t) => t.status === 'paid').reduce((acc, curr) => acc + (curr.amount_minor || 0), 0) ?? 0
  }, [topups])

  // Filtered Wallets
  const filteredWallets = useMemo(() => {
    if (!wallets?.data) return []
    if (!walletSearch.trim()) return wallets.data
    const query = walletSearch.toLowerCase().trim()
    return wallets.data.filter((w) => {
      const details = getUserDetails(w.user_id)
      return (
        String(w.user_id).includes(query) ||
        details.name.toLowerCase().includes(query) ||
        details.email.toLowerCase().includes(query)
      )
    })
  }, [wallets, walletSearch, getUserDetails])

  // Filtered Invoices
  const filteredInvoices = useMemo(() => {
    if (!invoices?.data) return []
    return invoices.data.filter((inv) => {
      const matchesStatus = invoiceStatusFilter === 'all' || inv.status === invoiceStatusFilter
      if (!matchesStatus) return false
      if (!invoiceSearch.trim()) return true
      const query = invoiceSearch.toLowerCase().trim()
      const details = getUserDetails(inv.user_id)
      return (
        String(inv.id).includes(query) ||
        String(inv.user_id).includes(query) ||
        details.name.toLowerCase().includes(query) ||
        details.email.toLowerCase().includes(query)
      )
    })
  }, [invoices, invoiceStatusFilter, invoiceSearch, getUserDetails])

  // Filtered Topups
  const filteredTopups = useMemo(() => {
    if (!topups?.data) return []
    return topups.data.filter((topup) => {
      const matchesStatus = topupStatusFilter === 'all' || topup.status === topupStatusFilter
      if (!matchesStatus) return false
      if (!topupSearch.trim()) return true
      const query = topupSearch.toLowerCase().trim()
      const details = getUserDetails(topup.user_id)
      return (
        String(topup.id).includes(query) ||
        String(topup.user_id).includes(query) ||
        details.name.toLowerCase().includes(query) ||
        details.email.toLowerCase().includes(query)
      )
    })
  }, [topups, topupStatusFilter, topupSearch, getUserDetails])

  // Search/status filters run client-side over the page the server returned, so
  // while one is active the server total would describe rows the table isn't
  // showing. Report the filtered count instead — the footer never contradicts
  // the body.
  // ponytail: real fix is a server-side `search` param; add when a page-local
  // filter stops being enough.
  const walletFiltered = walletSearch.trim() !== ''
  const invoiceFiltered = invoiceSearch.trim() !== '' || invoiceStatusFilter !== 'all'
  const topupFiltered = topupSearch.trim() !== '' || topupStatusFilter !== 'all'

  const walletPaging = serverPagination(
    walletFiltered ? 1 : walletPage,
    walletLimit,
    walletFiltered ? filteredWallets.length : (wallets?.total ?? 0),
    setWalletPage,
    setWalletLimit,
  )
  const invoicePaging = serverPagination(
    invoiceFiltered ? 1 : invoicePage,
    invoiceLimit,
    invoiceFiltered ? filteredInvoices.length : (invoices?.total ?? 0),
    setInvoicePage,
    setInvoiceLimit,
  )
  const topupPaging = serverPagination(
    topupFiltered ? 1 : topupPage,
    topupLimit,
    topupFiltered ? filteredTopups.length : (topups?.total ?? 0),
    setTopupPage,
    setTopupLimit,
  )

  return (
    <div className="mx-auto max-w-7xl space-y-6 pb-12">
      {/* Clean Anti-Slop Header */}
      <div className="flex flex-col gap-4 border-b border-border/60 pb-5 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-1.5 text-xs text-muted-foreground font-medium mb-1">
            <span>Admin</span>
            <ChevronRight className="size-3 text-muted-foreground/60" />
            <span className="text-foreground font-semibold">{t('billing.nav')}</span>
          </div>
          <h1 className="text-2xl font-bold tracking-tight text-foreground">{t('billing.admin.title')}</h1>
          <p className="mt-1 text-xs text-muted-foreground max-w-2xl">{t('billing.admin.description')}</p>
        </div>

        <div className="flex items-center gap-2 pt-1 sm:pt-0">
          {isSuperAdmin && (
            <Button
              size="sm"
              onClick={() => setIsDirectCreditModalOpen(true)}
              className="bg-primary hover:bg-primary/90 text-primary-foreground font-medium text-xs h-8 px-3 shadow-xs"
            >
              <Plus className="size-3.5 mr-1.5" />
              {t('billing.admin.addCredits')}
            </Button>
          )}
          <Button variant="outline" size="sm" onClick={() => void load()} disabled={loading} className="h-8 text-xs px-3">
            <RefreshCw className={`size-3.5 mr-1.5 ${loading ? 'animate-spin' : ''}`} />
            {t('billing.refresh')}
          </Button>
        </div>
      </div>

      {Object.keys(errors).length > 0 && (
        <Card className="border-amber-500/40 bg-amber-500/5" role="status">
          <CardContent className="pt-6 text-sm">{t('billing.loadPartial', { sections: Object.values(errors).join(', ') })}</CardContent>
        </Card>
      )}

      {/* KPI Overview Bar Header & Period Selector */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h2 className="text-base font-semibold tracking-tight text-foreground">Financial & Resource Performance</h2>
          <p className="text-xs text-muted-foreground">Real-time metrics and period aggregation trends</p>
        </div>

        <div className="flex items-center gap-2">
          <Select value={selectedPeriod} onValueChange={(val) => val && setSelectedPeriod(val)}>
            <SelectTrigger className="h-8 text-xs w-52">
              <Calendar className="size-3.5 mr-1.5 text-muted-foreground" />
              <span>
                {selectedPeriod === 'today' && 'Today (24h)'}
                {selectedPeriod === 'last_7_days' && 'Last 7 Days'}
                {selectedPeriod === 'last_30_days' && 'Last 30 Days'}
                {selectedPeriod === 'this_month' && 'This Month (Aug 2026)'}
                {selectedPeriod === 'last_month' && 'Last Month (Jul 2026)'}
                {selectedPeriod === 'last_3_months' && 'Last 3 Months'}
                {selectedPeriod === 'year_to_date' && 'Year to Date (2026)'}
                {selectedPeriod === 'custom' && 'Custom Date Range...'}
              </span>
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="today">Today (24h)</SelectItem>
              <SelectItem value="last_7_days">Last 7 Days</SelectItem>
              <SelectItem value="last_30_days">Last 30 Days</SelectItem>
              <SelectItem value="this_month">This Month (Aug 2026)</SelectItem>
              <SelectItem value="last_month">Last Month (Jul 2026)</SelectItem>
              <SelectItem value="last_3_months">Last 3 Months</SelectItem>
              <SelectItem value="year_to_date">Year to Date (2026)</SelectItem>
              <SelectItem value="custom">Custom Date Range...</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>

      {/* KPI Overview Grid with Sparklines & Dynamic Status */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {/* Stat Card 1: Wallets */}
        <Card className="relative overflow-hidden transition-all hover:border-border/80">
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              {t('billing.admin.wallets')}
            </CardTitle>
            <div className="flex size-8 items-center justify-center rounded-lg bg-primary/10 text-primary">
              <WalletCards className="size-4" />
            </div>
          </CardHeader>
          <CardContent className="space-y-1">
            <div className="flex items-baseline justify-between">
              <div className="text-2xl font-bold tracking-tight">
                {loading ? <Skeleton className="h-8 w-24" /> : formatCredits(totalCirculatingCredits)}
              </div>
              <span className="inline-flex items-center text-[10px] font-medium text-muted-foreground bg-muted border border-border/60 px-1.5 py-0.5 rounded-md">
                {wallets ? `${wallets.total} Active` : '—'}
              </span>
            </div>
            <p className="text-xs text-muted-foreground">
              {wallets ? `${wallets.total} user wallets tracked` : 'Loading wallets…'}
            </p>
            {/* Sparkline Visualizer */}
            <div className="pt-2">
              <svg className="w-full h-7 overflow-visible" viewBox="0 0 100 25">
                <defs>
                  <linearGradient id="grad-wallets" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor="currentColor" stopOpacity="0.3" className="text-primary" />
                    <stop offset="100%" stopColor="currentColor" stopOpacity="0" className="text-primary" />
                  </linearGradient>
                </defs>
                <path d="M0,20 Q15,18 30,12 T60,15 T80,5 T100,8 L100,25 L0,25 Z" fill="url(#grad-wallets)" />
                <path d="M0,20 Q15,18 30,12 T60,15 T80,5 T100,8" fill="none" stroke="currentColor" strokeWidth="1.5" className="text-primary" />
              </svg>
            </div>
          </CardContent>
        </Card>

        {/* Stat Card 2: Invoices */}
        <Card className="relative overflow-hidden transition-all hover:border-border/80">
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              {t('billing.invoices')}
            </CardTitle>
            <div className="flex size-8 items-center justify-center rounded-lg bg-blue-500/10 text-blue-500">
              <ReceiptText className="size-4" />
            </div>
          </CardHeader>
          <CardContent className="space-y-1">
            <div className="flex items-baseline justify-between">
              <div className="text-2xl font-bold tracking-tight">
                {loading ? <Skeleton className="h-8 w-16" /> : invoices?.total ?? 0}
              </div>
              <span className={`inline-flex items-center text-[10px] font-semibold px-1.5 py-0.5 rounded-md ${
                invoices && invoices.total > 0
                  ? 'text-blue-500 bg-blue-500/10 border border-blue-500/20'
                  : 'text-muted-foreground bg-muted border border-border/60'
              }`}>
                {invoices ? `${invoices.data.filter((i) => i.status === 'paid').length} Paid` : '0 Paid'}
              </span>
            </div>
            <p className="text-xs text-muted-foreground">
              {invoices ? `${invoices.data.filter((i) => i.status === 'paid').length} paid invoices` : 'Loading invoices…'}
            </p>
            {/* Sparkline Visualizer */}
            <div className="pt-2">
              <svg className="w-full h-7 overflow-visible" viewBox="0 0 100 25">
                <defs>
                  <linearGradient id="grad-invoices" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor="#3b82f6" stopOpacity="0.3" />
                    <stop offset="100%" stopColor="#3b82f6" stopOpacity="0" />
                  </linearGradient>
                </defs>
                <path d="M0,22 Q20,19 40,14 T70,9 T100,4 L100,25 L0,25 Z" fill="url(#grad-invoices)" />
                <path d="M0,22 Q20,19 40,14 T70,9 T100,4" fill="none" stroke="#3b82f6" strokeWidth="1.5" />
              </svg>
            </div>
          </CardContent>
        </Card>

        {/* Stat Card 3: Top-ups & Revenue */}
        <Card className="relative overflow-hidden transition-all hover:border-border/80">
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              {t('billing.topups')}
            </CardTitle>
            <div className="flex size-8 items-center justify-center rounded-lg bg-emerald-500/10 text-emerald-500">
              <CreditCard className="size-4" />
            </div>
          </CardHeader>
          <CardContent className="space-y-1">
            <div className="flex items-baseline justify-between">
              <div className="text-2xl font-bold tracking-tight">
                {loading ? <Skeleton className="h-8 w-28" /> : formatMoney(totalTopupsRevenue, 'IDR')}
              </div>
              <span className={`inline-flex items-center text-[10px] font-semibold px-1.5 py-0.5 rounded-md ${
                topups && totalTopupsRevenue > 0
                  ? 'text-emerald-500 bg-emerald-500/10 border border-emerald-500/20'
                  : 'text-muted-foreground bg-muted border border-border/60'
              }`}>
                {topups ? `${topups.total} Orders` : '0 Orders'}
              </span>
            </div>
            <p className="text-xs text-muted-foreground">
              {topups ? `${topups.total} top-ups processed` : 'Loading top-ups…'}
            </p>
            {/* Sparkline Visualizer */}
            <div className="pt-2">
              <svg className="w-full h-7 overflow-visible" viewBox="0 0 100 25">
                <defs>
                  <linearGradient id="grad-topups" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor="#10b981" stopOpacity="0.3" />
                    <stop offset="100%" stopColor="#10b981" stopOpacity="0" />
                  </linearGradient>
                </defs>
                <path d="M0,25 Q25,20 50,10 T80,6 T100,2 L100,25 L0,25 Z" fill="url(#grad-topups)" />
                <path d="M0,25 Q25,20 50,10 T80,6 T100,2" fill="none" stroke="#10b981" strokeWidth="1.5" />
              </svg>
            </div>
          </CardContent>
        </Card>

        {/* Stat Card 4: Suspensions */}
        <Card className={`relative overflow-hidden transition-all hover:border-border/80 ${suspensions && suspensions.length > 0 ? 'border-amber-500/40 bg-amber-500/5' : ''}`}>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              {t('billing.admin.suspensions')}
            </CardTitle>
            <div className={`flex size-8 items-center justify-center rounded-lg ${suspensions && suspensions.length > 0 ? 'bg-amber-500/20 text-amber-600 dark:text-amber-400' : 'bg-muted text-muted-foreground'}`}>
              <AlertTriangle className="size-4" />
            </div>
          </CardHeader>
          <CardContent className="space-y-1">
            <div className="flex items-baseline justify-between">
              <div className="text-2xl font-bold tracking-tight">
                {loading ? <Skeleton className="h-8 w-12" /> : suspensions?.length ?? 0}
              </div>
              <span className={`inline-flex items-center text-[10px] font-semibold px-1.5 py-0.5 rounded-md ${
                suspensions && suspensions.length > 0
                  ? 'text-amber-500 bg-amber-500/10 border border-amber-500/20'
                  : 'text-emerald-500 bg-emerald-500/10 border border-emerald-500/20'
              }`}>
                {suspensions && suspensions.length > 0 ? 'Action Required' : 'Healthy'}
              </span>
            </div>
            <p className="text-xs text-muted-foreground">
              {suspensions && suspensions.length > 0 ? 'Resources requiring payment' : 'No suspended resources'}
            </p>
            {/* Sparkline Visualizer (Flat Line for Healthy) */}
            <div className="pt-2">
              <svg className="w-full h-7 overflow-visible" viewBox="0 0 100 25">
                <path d="M0,20 L100,20" fill="none" stroke="currentColor" strokeWidth="1.5" strokeDasharray="3 3" className="text-muted-foreground/40" />
              </svg>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Suspensions Warning Section if any */}
      {suspensions && suspensions.length > 0 && (
        <Card className="border-amber-500/40 bg-amber-500/5">
          <CardHeader className="pb-3">
            <div className="flex items-center gap-2">
              <ShieldAlert className="size-5 text-amber-600 dark:text-amber-400" />
              <CardTitle className="text-base font-semibold">{t('billing.admin.suspensions')}</CardTitle>
            </div>
            <CardDescription className="text-xs text-amber-700 dark:text-amber-300">
              The following resources are currently suspended or marked as payment due.
            </CardDescription>
          </CardHeader>
          <CardContent className="p-0">
            <Table>
              <TableHeader>
                <TableRow className="border-amber-500/20 hover:bg-transparent">
                  <TableHead className="pl-4 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">User</TableHead>
                  <TableHead className="w-[200px] text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Resource</TableHead>
                  <TableHead className="w-[140px] text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Oldest Due Date</TableHead>
                  <TableHead className="w-[130px] text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Overdue Days</TableHead>
                  <TableHead className="w-[130px] pr-4 text-right text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Status</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {suspensions.map((item) => {
                  const userDetails = getUserDetails(item.user_id)
                  return (
                    <TableRow key={`${item.user_id}-${item.resource_type}-${item.resource_id}`} className="border-amber-500/20">
                      <TableCell className="py-2 pl-4 font-medium">
                        <div className="flex items-center gap-2">
                          <div className="flex size-7 shrink-0 items-center justify-center rounded-full bg-amber-500/20 text-xs font-bold text-amber-700 dark:text-amber-300">
                            {userDetails.initials}
                          </div>
                          <div>
                            <div className="text-xs font-semibold">{userDetails.name}</div>
                            <div className="text-[10px] text-muted-foreground">{userDetails.email || `#${item.user_id}`}</div>
                          </div>
                        </div>
                      </TableCell>
                      <TableCell>
                        <Badge variant="outline" className="capitalize text-xs font-normal">
                          {t(`billing.resourceTypes.${item.resource_type}`) !== `billing.resourceTypes.${item.resource_type}`
                            ? t(`billing.resourceTypes.${item.resource_type}`)
                            : item.resource_type}{' '}
                          #{item.resource_id}
                        </Badge>
                      </TableCell>
                      <TableCell className="py-2 text-xs tabular-nums text-muted-foreground">{formatDate(item.oldest_due_at)}</TableCell>
                      <TableCell className="py-2 text-xs font-medium tabular-nums">{t('billing.days', { count: item.payment_due_days })}</TableCell>
                      <TableCell className="py-2 pr-4 text-right">
                        <Badge variant={statusVariant(item.status)}>{formatStatus(item.status)}</Badge>
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}

      {/* Main Tabbed Data Center */}
      <Tabs defaultValue="wallets" className="w-full space-y-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between border-b pb-2">
          <TabsList variant="line" className="h-9">
            <TabsTrigger value="wallets" className="gap-2 text-xs font-medium">
              <WalletCards className="size-3.5" />
              {t('billing.admin.wallets')}
              {wallets && <Badge variant="secondary" className="ml-1 px-1.5 py-0 text-[10px]">{wallets.total}</Badge>}
            </TabsTrigger>
            <TabsTrigger value="invoices" className="gap-2 text-xs font-medium">
              <ReceiptText className="size-3.5" />
              {t('billing.invoices')}
              {invoices && <Badge variant="secondary" className="ml-1 px-1.5 py-0 text-[10px]">{invoices.total}</Badge>}
            </TabsTrigger>
            <TabsTrigger value="topups" className="gap-2 text-xs font-medium">
              <CreditCard className="size-3.5" />
              {t('billing.topups')}
              {topups && <Badge variant="secondary" className="ml-1 px-1.5 py-0 text-[10px]">{topups.total}</Badge>}
            </TabsTrigger>
            <TabsTrigger value="catalog" className="gap-2 text-xs font-medium">
              <Layers className="size-3.5" />
              Plans & Packages
              {catalog && <Badge variant="secondary" className="ml-1 px-1.5 py-0 text-[10px]">{catalog.specs.length + catalog.packages.length}</Badge>}
            </TabsTrigger>
          </TabsList>
        </div>

        {/* ================= WALLETS TAB ================= */}
        <TabsContent value="wallets" className="space-y-4">
          <Card>
            <CardHeader className="flex flex-col gap-3 pb-3 sm:flex-row sm:items-center sm:justify-between">
              <div>
                <CardTitle className="text-base font-semibold">{t('billing.admin.wallets')}</CardTitle>
                <CardDescription className="text-xs">User credit balance activity and manual adjustment</CardDescription>
              </div>

              <div className="flex items-center gap-2">
                <div className="relative min-w-0 w-full sm:w-64">
                  <Search className="absolute left-2.5 top-2.5 size-4 text-muted-foreground" />
                  <Input
                    placeholder="Search name, email, or user ID…"
                    value={walletSearch}
                    onChange={(e) => setWalletSearch(e.target.value)}
                    className="pl-8 h-9 text-xs"
                  />
                </div>
              </div>
            </CardHeader>
            <CardContent className="p-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="pl-4 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">User Account</TableHead>
                    <TableHead className="w-[120px] text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Role</TableHead>
                    <TableHead className="w-[180px] text-right text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Credit Balance</TableHead>
                    <TableHead className="w-[140px] text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Last Updated</TableHead>
                    <TableHead className="w-[140px] pr-4 text-right text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Action</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {loading && Array.from({ length: walletPaging.pageSize }).map((_, i) => (
                    <TableRow key={i}>
                      <TableCell><Skeleton className="h-9 w-48" /></TableCell>
                      <TableCell><Skeleton className="h-5 w-16" /></TableCell>
                      <TableCell className="text-right"><Skeleton className="h-5 w-24 ml-auto" /></TableCell>
                      <TableCell><Skeleton className="h-5 w-28" /></TableCell>
                      <TableCell className="text-right"><Skeleton className="h-8 w-24 ml-auto" /></TableCell>
                    </TableRow>
                  ))}

                  {!loading && errors.wallets && (
                    <TableRow>
                      <TableCell colSpan={5} className="py-8 text-center text-sm text-destructive">
                        {t('billing.loadError', { section: t('billing.admin.wallets') })}
                        <Button size="sm" variant="outline" className="ml-3" onClick={() => void load()}>
                          {t('billing.retry')}
                        </Button>
                      </TableCell>
                    </TableRow>
                  )}

                  {!loading && filteredWallets.map((wallet) => {
                    const userDetails = getUserDetails(wallet.user_id)
                    return (
                      <TableRow key={wallet.user_id} className="transition-colors hover:bg-muted/40">
                        <TableCell className="py-2 pl-4">
                          <div className="flex items-center gap-2.5">
                            <div className="flex size-8 shrink-0 items-center justify-center rounded-full bg-primary/10 text-[11px] font-semibold text-primary">
                              {userDetails.initials}
                            </div>
                            <div className="min-w-0">
                              <div className="flex items-center gap-1.5">
                                <span className="truncate text-xs font-semibold">{userDetails.name}</span>
                                <span className="shrink-0 font-mono text-[10px] text-muted-foreground">#{wallet.user_id}</span>
                              </div>
                              <div className="truncate text-[11px] text-muted-foreground">{userDetails.email || 'No email registered'}</div>
                            </div>
                          </div>
                        </TableCell>

                        <TableCell className="py-2">
                          {userDetails.found ? (
                            <Badge variant="secondary" className="text-[10px] font-medium capitalize">
                              {userDetails.role}
                            </Badge>
                          ) : (
                            <span className="text-xs text-muted-foreground">—</span>
                          )}
                        </TableCell>

                        <TableCell className="py-2 text-right">
                          <span className="text-xs font-semibold tabular-nums text-foreground">
                            {formatCredits(wallet.balance_credits)}
                          </span>
                          <span className="ml-1 text-[11px] text-muted-foreground">{t('billing.credits')}</span>
                        </TableCell>

                        <TableCell className="py-2 text-[11px] tabular-nums text-muted-foreground">
                          {formatDate(wallet.updated_at)}
                        </TableCell>

                        <TableCell className="py-2 pr-4 text-right">
                          {isSuperAdmin && (
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => startAdjustWallet(wallet)}
                              className="h-8 text-xs font-medium"
                            >
                              <Plus className="size-3.5 mr-1" />
                              {t('billing.admin.addCredits')}
                            </Button>
                          )}
                        </TableCell>
                      </TableRow>
                    )
                  })}

                  {!loading && filteredWallets.length === 0 && (
                    <TableRow>
                      <TableCell colSpan={5} className="py-8 text-center text-sm text-muted-foreground">
                        {t('billing.admin.noRecords')}
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </CardContent>

            {/* Pagination Footer */}
            <TablePagination state={walletPaging} disabled={loading} />
          </Card>
        </TabsContent>

        {/* ================= INVOICES TAB ================= */}
        <TabsContent value="invoices" className="space-y-4">
          <Card>
            <CardHeader className="flex flex-col gap-3 pb-3 sm:flex-row sm:items-center sm:justify-between">
              <div>
                <CardTitle className="text-base font-semibold">{t('billing.invoices')}</CardTitle>
                <CardDescription className="text-xs">Generated monthly resource usage invoices</CardDescription>
              </div>

              <div className="flex items-center gap-2">
                <div className="relative min-w-0 flex-1 sm:max-w-60">
                  <Search className="absolute left-2.5 top-2.5 size-4 text-muted-foreground" />
                  <Input
                    placeholder="Search invoice or user…"
                    value={invoiceSearch}
                    onChange={(e) => setInvoiceSearch(e.target.value)}
                    className="pl-8 h-9 text-xs"
                  />
                </div>

                <Select value={invoiceStatusFilter} onValueChange={(val) => val && setInvoiceStatusFilter(val)}>
                  <SelectTrigger className="h-9 w-36 shrink-0 text-xs">
                    <Filter className="size-3.5 mr-1 text-muted-foreground" />
                    <span className="capitalize">{invoiceStatusFilter === 'all' ? 'All Statuses' : formatStatus(invoiceStatusFilter)}</span>
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">All Statuses</SelectItem>
                    <SelectItem value="paid">Paid</SelectItem>
                    <SelectItem value="payment_due">Payment Due</SelectItem>
                    <SelectItem value="void">Void</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </CardHeader>

            <CardContent className="p-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-[110px] pl-4 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Invoice ID</TableHead>
                    <TableHead className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">User Account</TableHead>
                    <TableHead className="w-[130px] text-right text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Credits</TableHead>
                    <TableHead className="w-[230px] text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Billing Period</TableHead>
                    <TableHead className="w-[130px] text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Paid / Due</TableHead>
                    <TableHead className="w-[110px] text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Status</TableHead>
                    <TableHead className="w-[80px] pr-4 text-right text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Detail</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {loading && Array.from({ length: invoicePaging.pageSize }).map((_, i) => (
                    <TableRow key={i}>
                      <TableCell><Skeleton className="h-5 w-16" /></TableCell>
                      <TableCell><Skeleton className="h-9 w-48" /></TableCell>
                      <TableCell className="text-right"><Skeleton className="h-5 w-20 ml-auto" /></TableCell>
                      <TableCell><Skeleton className="h-5 w-36" /></TableCell>
                      <TableCell><Skeleton className="h-5 w-24" /></TableCell>
                      <TableCell><Skeleton className="h-5 w-16" /></TableCell>
                      <TableCell className="text-right"><Skeleton className="h-5 w-12 ml-auto" /></TableCell>
                    </TableRow>
                  ))}

                  {!loading && errors.invoices && (
                    <TableRow>
                      <TableCell colSpan={7} className="py-8 text-center text-sm text-destructive">
                        {t('billing.loadError', { section: t('billing.invoices') })}
                        <Button size="sm" variant="outline" className="ml-3" onClick={() => void load()}>
                          {t('billing.retry')}
                        </Button>
                      </TableCell>
                    </TableRow>
                  )}

                  {!loading && filteredInvoices.map((invoice) => {
                    const userDetails = getUserDetails(invoice.user_id)
                    return (
                      <TableRow key={invoice.id} className="transition-colors hover:bg-muted/40">
                        <TableCell className="py-2 pl-4 font-mono text-[11px] font-semibold text-foreground">
                          #INV-{invoice.id}
                        </TableCell>

                        <TableCell className="py-2">
                          <div className="flex items-center gap-2.5">
                            <div className="flex size-8 shrink-0 items-center justify-center rounded-full bg-blue-500/10 text-[11px] font-semibold text-blue-500">
                              {userDetails.initials}
                            </div>
                            <div className="min-w-0">
                              <div className="truncate text-xs font-semibold">{userDetails.name}</div>
                              <div className="truncate text-[11px] text-muted-foreground">{userDetails.email || `#${invoice.user_id}`}</div>
                            </div>
                          </div>
                        </TableCell>

                        <TableCell className="py-2 text-right">
                          <span className="text-xs font-semibold tabular-nums">{formatCredits(invoice.total_credits)}</span>
                          <span className="ml-1 text-[11px] text-muted-foreground">{t('billing.credits')}</span>
                        </TableCell>

                        <TableCell className="py-2 text-[11px] tabular-nums text-muted-foreground">
                          {formatDate(invoice.period_start)} – {formatDate(invoice.period_end)}
                        </TableCell>

                        <TableCell className="py-2 text-[11px] tabular-nums text-muted-foreground">
                          {formatDate(invoice.paid_at || invoice.due_at)}
                        </TableCell>

                        <TableCell className="py-2">
                          <Badge variant={statusVariant(invoice.status)} className="text-[10px]">
                            {formatStatus(invoice.status)}
                          </Badge>
                        </TableCell>

                        <TableCell className="py-2 pr-4 text-right">
                          <Button
                            size="sm"
                            variant="ghost"
                            className="h-7 px-2 text-[11px] font-medium"
                            onClick={() => setViewingInvoice(invoice)}
                          >
                            {t('billing.viewReceipt')}
                          </Button>
                        </TableCell>
                      </TableRow>
                    )
                  })}

                  {!loading && filteredInvoices.length === 0 && (
                    <TableRow>
                      <TableCell colSpan={7} className="py-8 text-center text-sm text-muted-foreground">
                        {t('billing.admin.noRecords')}
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </CardContent>

            <TablePagination state={invoicePaging} disabled={loading} />
          </Card>
        </TabsContent>

        {/* ================= TOP-UPS TAB ================= */}
        <TabsContent value="topups" className="space-y-4">
          <Card>
            <CardHeader className="flex flex-col gap-3 pb-3 sm:flex-row sm:items-center sm:justify-between">
              <div>
                <CardTitle className="text-base font-semibold">{t('billing.topups')}</CardTitle>
                <CardDescription className="text-xs">Purchased credit package payments and status</CardDescription>
              </div>

              <div className="flex items-center gap-2">
                <div className="relative min-w-0 flex-1 sm:max-w-60">
                  <Search className="absolute left-2.5 top-2.5 size-4 text-muted-foreground" />
                  <Input
                    placeholder="Search top-up or user…"
                    value={topupSearch}
                    onChange={(e) => setTopupSearch(e.target.value)}
                    className="pl-8 h-9 text-xs"
                  />
                </div>

                <Select value={topupStatusFilter} onValueChange={(val) => val && setTopupStatusFilter(val)}>
                  <SelectTrigger className="h-9 w-36 shrink-0 text-xs">
                    <Filter className="size-3.5 mr-1 text-muted-foreground" />
                    <span className="capitalize">{topupStatusFilter === 'all' ? 'All Statuses' : formatStatus(topupStatusFilter)}</span>
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">All Statuses</SelectItem>
                    <SelectItem value="paid">Paid</SelectItem>
                    <SelectItem value="pending">Pending</SelectItem>
                    <SelectItem value="expired">Expired</SelectItem>
                    <SelectItem value="refunded">Refunded</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </CardHeader>

            <CardContent className="p-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-[110px] pl-4 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Top-up ID</TableHead>
                    <TableHead className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">User Account</TableHead>
                    <TableHead className="w-[120px] text-right text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Credits</TableHead>
                    <TableHead className="w-[150px] text-right text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Amount Paid</TableHead>
                    <TableHead className="w-[130px] text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Date</TableHead>
                    <TableHead className="w-[110px] pr-4 text-right text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Status</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {loading && Array.from({ length: topupPaging.pageSize }).map((_, i) => (
                    <TableRow key={i}>
                      <TableCell><Skeleton className="h-5 w-16" /></TableCell>
                      <TableCell><Skeleton className="h-9 w-48" /></TableCell>
                      <TableCell className="text-right"><Skeleton className="h-5 w-20 ml-auto" /></TableCell>
                      <TableCell className="text-right"><Skeleton className="h-5 w-24 ml-auto" /></TableCell>
                      <TableCell><Skeleton className="h-5 w-28" /></TableCell>
                      <TableCell className="text-right"><Skeleton className="h-5 w-16 ml-auto" /></TableCell>
                    </TableRow>
                  ))}

                  {!loading && errors.topups && (
                    <TableRow>
                      <TableCell colSpan={6} className="py-8 text-center text-sm text-destructive">
                        {t('billing.loadError', { section: t('billing.topups') })}
                        <Button size="sm" variant="outline" className="ml-3" onClick={() => void load()}>
                          {t('billing.retry')}
                        </Button>
                      </TableCell>
                    </TableRow>
                  )}

                  {!loading && filteredTopups.map((topup) => {
                    const userDetails = getUserDetails(topup.user_id)
                    return (
                      <TableRow key={topup.id} className="transition-colors hover:bg-muted/40">
                        <TableCell className="py-2 pl-4 font-mono text-[11px] font-semibold text-foreground">
                          #TOP-{topup.id}
                        </TableCell>

                        <TableCell className="py-2">
                          <div className="flex items-center gap-2.5">
                            <div className="flex size-8 shrink-0 items-center justify-center rounded-full bg-emerald-500/10 text-[11px] font-semibold text-emerald-500">
                              {userDetails.initials}
                            </div>
                            <div className="min-w-0">
                              <div className="truncate text-xs font-semibold">{userDetails.name}</div>
                              <div className="truncate text-[11px] text-muted-foreground">{userDetails.email || `#${topup.user_id}`}</div>
                            </div>
                          </div>
                        </TableCell>

                        <TableCell className="py-2 text-right">
                          <span className="text-xs font-semibold tabular-nums">{formatCredits(topup.credits)}</span>
                          <span className="ml-1 text-[11px] text-muted-foreground">{t('billing.credits')}</span>
                        </TableCell>

                        <TableCell className="py-2 text-right text-xs font-semibold tabular-nums text-emerald-600 dark:text-emerald-400">
                          {formatMoney(topup.amount_minor, topup.currency)}
                        </TableCell>

                        <TableCell className="py-2 text-[11px] tabular-nums text-muted-foreground">
                          {formatDate(topup.paid_at || topup.created_at)}
                        </TableCell>

                        <TableCell className="py-2 pr-4 text-right">
                          <Badge variant={statusVariant(topup.status)} className="text-[10px]">
                            {formatStatus(topup.status)}
                          </Badge>
                        </TableCell>
                      </TableRow>
                    )
                  })}

                  {!loading && filteredTopups.length === 0 && (
                    <TableRow>
                      <TableCell colSpan={6} className="py-8 text-center text-sm text-muted-foreground">
                        {t('billing.admin.noRecords')}
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </CardContent>

            <TablePagination state={topupPaging} disabled={loading} />
          </Card>
        </TabsContent>

        {/* ================= CATALOG & PACKAGES TAB ================= */}
        <TabsContent value="catalog" className="space-y-6">
          <div className="grid gap-6 xl:grid-cols-2">
            {/* Resource Plans */}
            <Card className="h-full">
              <CardHeader className="flex flex-col gap-3 pb-3">
                <div className="min-w-0">
                  <CardTitle className="text-base font-semibold flex items-center gap-2">
                    <Boxes className="size-4 text-primary" />
                    {t('billing.admin.resourcePlans')}
                  </CardTitle>
                  <CardDescription className="text-xs">Active and historical compute resource pricing specs</CardDescription>
                </div>
                <div className="flex items-center gap-2">
                  <div className="relative min-w-0 flex-1">
                    <Search className="absolute left-2.5 top-2.5 size-3.5 text-muted-foreground" />
                    <Input
                      placeholder="Search spec…"
                      value={specSearch}
                      onChange={(e) => setSpecSearch(e.target.value)}
                      className="pl-8 h-8 text-xs"
                    />
                  </div>
                  <Select value={specTypeFilter} onValueChange={(v) => v && setSpecTypeFilter(v)}>
                    <SelectTrigger className="h-8 w-28 shrink-0 text-xs">
                      <span className="capitalize">{specTypeFilter === 'all' ? 'All Types' : specTypeFilter}</span>
                    </SelectTrigger>
                    <SelectContent side="bottom" align="end">
                      <SelectItem value="all">All Types</SelectItem>
                      <SelectItem value="project">Project</SelectItem>
                      <SelectItem value="database">Database</SelectItem>
                    </SelectContent>
                  </Select>
                  <Select value={specActiveFilter} onValueChange={(v) => v && setSpecActiveFilter(v)}>
                    <SelectTrigger className="h-8 w-28 shrink-0 text-xs">
                      <span className="capitalize">{specActiveFilter}</span>
                    </SelectTrigger>
                    <SelectContent side="bottom" align="end">
                      <SelectItem value="active">Active</SelectItem>
                      <SelectItem value="inactive">Inactive</SelectItem>
                      <SelectItem value="all">All Status</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </CardHeader>
              <CardContent className="flex-1 space-y-3">
                {loading && Array.from({ length: specPaging.pageSize }).map((_, i) => <Skeleton key={i} className="h-16 w-full" />)}
                {!loading && errors.plans && (
                  <p className="text-sm text-destructive">{t('billing.loadError', { section: t('billing.admin.resourcePlans') })}</p>
                )}
                {!loading && !errors.plans && sortedSpecs.length === 0 && (
                  <p className="py-8 text-center text-sm text-muted-foreground">{t('billing.admin.noRecords')}</p>
                )}
                {!loading &&
                  sortedSpecs
                    .slice(specPaging.start, specPaging.end)
                    .map((spec) => (
                      <div key={spec.id} className="flex items-start justify-between gap-3 border-b pb-3 last:border-0">
                        <div>
                          <div className="flex items-center gap-2">
                            <span className="text-sm font-semibold text-foreground">{spec.name}</span>
                            <Badge variant="outline" className="text-[10px] capitalize">
                              {t(`billing.resourceTypes.${spec.type}`)}
                            </Badge>
                            {spec.badge_text && <Badge className="text-[10px]">{spec.badge_text}</Badge>}
                          </div>
                          <p className="text-xs font-semibold tabular-nums text-primary mt-0.5">
                            {formatCredits(spec.monthly_credits)} {t('billing.credits')} / mo
                          </p>
                          <p className="text-xs text-muted-foreground mt-1">
                            v{spec.version} • {spec.cpu_millicores}m CPU • {spec.memory_mb} MB RAM • {spec.storage_gb} GB SSD
                            {spec.connection_limit ? ` • ${spec.connection_limit} conn` : ''}
                          </p>
                        </div>
                        <div className="flex items-center gap-2">
                          {isSuperAdmin && spec.is_active && (
                            <Button size="icon-xs" variant="ghost" onClick={() => editSpec(spec.id)} aria-label="Edit Spec">
                              <Pencil className="size-3.5" />
                            </Button>
                          )}
                          <Badge variant={spec.is_active ? 'secondary' : 'outline'}>
                            {spec.is_active ? t('billing.admin.active') : t('billing.admin.inactive')}
                          </Badge>
                        </div>
                      </div>
                    ))}
              </CardContent>
              <TablePagination state={specPaging} disabled={loading} />
            </Card>

            {/* Topup Packages */}
            <Card className="h-full">
              <CardHeader className="flex flex-col gap-3 pb-3">
                <div className="min-w-0">
                  <CardTitle className="text-base font-semibold flex items-center gap-2">
                    <Coins className="size-4 text-emerald-500" />
                    {t('billing.admin.topupPackages')}
                  </CardTitle>
                  <CardDescription className="text-xs font-normal">Purchasable credit bundle offers</CardDescription>
                </div>
                <div className="flex items-center gap-2">
                  <div className="relative min-w-0 flex-1">
                    <Search className="absolute left-2.5 top-2.5 size-3.5 text-muted-foreground" />
                    <Input
                      placeholder="Search amount…"
                      value={packageSearch}
                      onChange={(e) => setPackageSearch(e.target.value)}
                      className="pl-8 h-8 text-xs"
                    />
                  </div>
                  <Select value={packageActiveFilter} onValueChange={(v) => v && setPackageActiveFilter(v)}>
                    <SelectTrigger className="h-8 w-28 shrink-0 text-xs">
                      <span className="capitalize">{packageActiveFilter}</span>
                    </SelectTrigger>
                    <SelectContent side="bottom" align="end">
                      <SelectItem value="active">Active</SelectItem>
                      <SelectItem value="inactive">Inactive</SelectItem>
                      <SelectItem value="all">All Status</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </CardHeader>
              <CardContent className="flex-1 space-y-3">
                {loading && Array.from({ length: packagePaging.pageSize }).map((_, i) => <Skeleton key={i} className="h-16 w-full" />)}
                {!loading && errors.plans && (
                  <p className="text-sm text-destructive">{t('billing.loadError', { section: t('billing.admin.topupPackages') })}</p>
                )}
                {!loading && !errors.plans && sortedPackages.length === 0 && (
                  <p className="py-8 text-center text-sm text-muted-foreground">{t('billing.admin.noRecords')}</p>
                )}
                {!loading &&
                  sortedPackages
                    .slice(packagePaging.start, packagePaging.end)
                    .map((pkg) => (
                      <div key={pkg.id} className="flex items-center justify-between gap-3 border-b pb-3 last:border-0">
                        <div>
                          <div className="text-sm font-semibold tabular-nums text-foreground">
                            {formatCredits(pkg.credits)} {t('billing.credits')}
                          </div>
                          <div className="text-xs font-semibold tabular-nums text-emerald-600 dark:text-emerald-400">
                            {formatMoney(pkg.amount_minor, pkg.currency)}
                          </div>
                          <div className="text-[10px] tabular-nums text-muted-foreground mt-0.5">
                            Version v{pkg.version} • Order #{pkg.sort_order}
                          </div>
                        </div>
                        <div className="flex items-center gap-2">
                          {isSuperAdmin && pkg.is_active && (
                            <Button size="icon-xs" variant="ghost" onClick={() => startEditPackage(pkg)} aria-label="Edit Package">
                              <Pencil className="size-3.5" />
                            </Button>
                          )}
                          <Badge variant={pkg.is_active ? 'secondary' : 'outline'}>
                            {pkg.is_active ? t('billing.admin.active') : t('billing.admin.inactive')}
                          </Badge>
                        </div>
                      </div>
                    ))}
              </CardContent>
              <TablePagination state={packagePaging} disabled={loading} />
            </Card>
          </div>
        </TabsContent>
      </Tabs>

      {/* ================= SUPERADMIN FORM CONTROL SECTION ================= */}
      {isSuperAdmin ? (
        <div className="mt-8 space-y-6">
          <div className="border-t border-border/60 pt-6">
            <h2 className="text-lg font-bold tracking-tight">Pricing Catalog & System Management</h2>
            <p className="text-xs text-muted-foreground">Superadmin operations for versioned compute pricing and top-up package management.</p>
          </div>

          <div className="grid gap-6 lg:grid-cols-2">
            {/* Create Spec Form */}
            <PricingForm
              title={editingSpec ? t('billing.admin.editPlan') : t('billing.admin.createPlan')}
              description={t('billing.admin.versionedPricing')}
              onSubmit={createSpec}
              submitting={creatingSpec}
              submitLabel={editingSpec ? t('billing.admin.saveNewVersion') : t('billing.admin.create')}
              secondaryAction={
                editingSpec && (
                  <Button type="button" variant="ghost" size="sm" onClick={() => { setEditingSpec(null); setSpecForm(emptySpec) }}>
                    {t('common.cancel')}
                  </Button>
                )
              }
            >
              {editingSpec && (
                <div className="flex items-center justify-between rounded-lg border border-primary/20 bg-primary/5 px-3 py-2 text-xs text-primary">
                  <span>{t('billing.admin.editingPlan', { name: editingSpec.name, version: editingSpec.version })}</span>
                  <Badge variant="outline" className="text-[10px] font-mono">v{editingSpec.version}</Badge>
                </div>
              )}
              <div className="grid gap-3 sm:grid-cols-2">
                <Field label={t('billing.admin.resourceType')} htmlFor="spec-type">
                  <Select
                    name="type"
                    value={specForm.type}
                    onValueChange={(type) => setSpecForm((current) => ({ ...current, type: type as 'project' | 'database', connection_limit: '', backup_retention_days: '' }))}
                    disabled={Boolean(editingSpec)}
                  >
                    <SelectTrigger id="spec-type" className="w-full h-9 text-xs">
                      {specForm.type === 'project' ? t('billing.admin.project') : t('billing.admin.database')}
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="project">{t('billing.admin.project')}</SelectItem>
                      <SelectItem value="database">{t('billing.admin.database')}</SelectItem>
                    </SelectContent>
                  </Select>
                </Field>

                <NumberField
                  id="spec-monthly-credits"
                  label={t('billing.admin.monthlyCredits')}
                  value={specForm.monthly_credits}
                  onChange={(monthly_credits) => setSpecForm((current) => ({ ...current, monthly_credits }))}
                  min={1}
                  placeholder="200"
                  hint={typeof specForm.monthly_credits === 'number' && specForm.monthly_credits > 0 ? t('billing.admin.monthlyCreditsHint', { price: formatMoney(specForm.monthly_credits * 1000, 'IDR') }) : undefined}
                />

                <TextField
                  id="spec-name"
                  label={t('billing.admin.name')}
                  value={specForm.name}
                  onChange={(name) =>
                    setSpecForm((current) => ({
                      ...current,
                      name,
                      slug: !current.slug || current.slug === slugify(current.name) ? slugify(name) : current.slug,
                    }))
                  }
                  placeholder="Large"
                  required
                  disabled={Boolean(editingSpec)}
                />

                <TextField
                  id="spec-slug"
                  label={t('billing.admin.slug')}
                  value={specForm.slug}
                  onChange={(slug) => setSpecForm((current) => ({ ...current, slug }))}
                  placeholder="project-large"
                  pattern="[a-z0-9-]+"
                  required
                  disabled={Boolean(editingSpec)}
                  hint={t('billing.admin.slugHint')}
                />

                <TextField
                  id="spec-badge-text"
                  label={t('billing.admin.badgeText')}
                  value={specForm.badge_text}
                  onChange={(badge_text) => setSpecForm((current) => ({ ...current, badge_text }))}
                  placeholder="Popular"
                />

                <NumberField id="spec-cpu" label={t('billing.admin.cpu')} value={specForm.cpu_millicores} onChange={(cpu_millicores) => setSpecForm((current) => ({ ...current, cpu_millicores }))} min={1} placeholder="1000" hint={t('billing.admin.cpuHint')} />
                <NumberField id="spec-memory" label={t('billing.admin.memory')} value={specForm.memory_mb} onChange={(memory_mb) => setSpecForm((current) => ({ ...current, memory_mb }))} min={1} placeholder="2048" />
                <NumberField id="spec-storage" label={t('billing.admin.storage')} value={specForm.storage_gb} onChange={(storage_gb) => setSpecForm((current) => ({ ...current, storage_gb }))} min={1} placeholder="20" />

                {specForm.type === 'database' && (
                  <>
                    <Field label={t('billing.admin.connectionLimit')} htmlFor="spec-connection-limit">
                      <Input id="spec-connection-limit" type="number" min={1} max={1000000} value={specForm.connection_limit} onChange={(event) => setSpecForm((current) => ({ ...current, connection_limit: event.target.value === '' ? '' : Number(event.target.value) }))} placeholder="100" className="h-9 text-xs" />
                    </Field>
                    <Field label={t('billing.admin.backupRetentionDays')} htmlFor="spec-backup-retention">
                      <Input id="spec-backup-retention" type="number" min={1} max={3650} value={specForm.backup_retention_days} onChange={(event) => setSpecForm((current) => ({ ...current, backup_retention_days: event.target.value === '' ? '' : Number(event.target.value) }))} placeholder="7" className="h-9 text-xs" />
                    </Field>
                  </>
                )}
              </div>

              <Field label={t('billing.admin.reason')} htmlFor="spec-reason" hint={t('billing.admin.reasonHint')}>
                <Textarea id="spec-reason" value={specForm.reason} onChange={(event) => setSpecForm((current) => ({ ...current, reason: event.target.value }))} placeholder={t('billing.admin.reasonPlaceholder')} required className="min-h-[80px] text-xs" />
              </Field>
            </PricingForm>

            {/* Create Package Form */}
            <PricingForm title={t('billing.admin.createPackage')} description={t('billing.admin.versionedPricing')} onSubmit={createPackage} submitting={creatingPackage} submitLabel={t('billing.admin.create')}>
              <div className="grid gap-3 sm:grid-cols-2">
                <NumberField id="package-credits" label={t('billing.admin.credits')} value={packageForm.credits} onChange={(credits) => setPackageForm((current) => ({ ...current, credits }))} min={1} placeholder="100" />
                
                <Field label={t('billing.admin.currency')} htmlFor="package-currency">
                  <Select name="currency" value={packageForm.currency} onValueChange={(currency) => setPackageForm((current) => ({ ...current, currency: currency || 'IDR' }))}>
                    <SelectTrigger id="package-currency" className="w-full h-9 text-xs">{packageForm.currency}</SelectTrigger>
                    <SelectContent>{SUPPORTED_CURRENCIES.map((code) => <SelectItem key={code} value={code}>{code}</SelectItem>)}</SelectContent>
                  </Select>
                </Field>

                <NumberField id="package-amount" label={t('billing.admin.packagePrice')} value={packageForm.amount_minor} onChange={(amount_minor) => setPackageForm((current) => ({ ...current, amount_minor }))} min={1} placeholder="100000" />
                <NumberField id="package-sort-order" label={t('billing.admin.sortOrder')} value={packageForm.sort_order} onChange={(sort_order) => setPackageForm((current) => ({ ...current, sort_order }))} min={0} placeholder="1" />
              </div>

              {typeof packageForm.amount_minor === 'number' && packageForm.amount_minor > 0 && (
                <div className="flex items-center gap-2 rounded-md border border-emerald-500/20 bg-emerald-500/5 px-3 py-2 text-xs text-emerald-600 dark:text-emerald-400">
                  <Coins className="size-3.5" />
                  <span>Calculated Price: <strong>{formatMoney(packageForm.amount_minor, packageForm.currency)}</strong></span>
                </div>
              )}

              <Field label={t('billing.admin.reason')} htmlFor="package-reason">
                <Textarea id="package-reason" value={packageForm.reason} onChange={(event) => setPackageForm((current) => ({ ...current, reason: event.target.value }))} placeholder="Explain why this top-up package is being created..." required className="min-h-[80px] text-xs" />
              </Field>
            </PricingForm>
          </div>
        </div>
      ) : (
        <Card>
          <CardContent className="pt-6 text-sm text-muted-foreground">{t('billing.admin.superadminOnly')}</CardContent>
        </Card>
      )}

      {/* Edit Package Dialog */}
      <Dialog open={editingPackage !== null} onOpenChange={(open) => !open && setEditingPackage(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('billing.admin.editPackage')}</DialogTitle>
            <DialogDescription>{t('billing.admin.editPackageDescription')}</DialogDescription>
          </DialogHeader>
          <form className="space-y-4" onSubmit={savePackageEdit}>
            <div className="grid gap-3 sm:grid-cols-2">
              <NumberField id="edit-package-credits" label={t('billing.admin.credits')} value={packageEditForm.credits} onChange={(credits) => setPackageEditForm((current) => ({ ...current, credits }))} min={1} />
              <NumberField id="edit-package-amount" label={t('billing.admin.packagePrice')} value={packageEditForm.amount_minor} onChange={(amount_minor) => setPackageEditForm((current) => ({ ...current, amount_minor }))} min={1} />
              <NumberField id="edit-package-sort-order" label={t('billing.admin.sortOrder')} value={packageEditForm.sort_order} onChange={(sort_order) => setPackageEditForm((current) => ({ ...current, sort_order }))} min={0} />
            </div>
            <Field label={t('billing.admin.reason')} htmlFor="edit-package-reason">
              <Textarea id="edit-package-reason" value={packageEditForm.reason} onChange={(event) => setPackageEditForm((current) => ({ ...current, reason: event.target.value }))} required />
            </Field>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setEditingPackage(null)}>{t('common.cancel')}</Button>
              <Button type="submit" disabled={savingPackageEdit}>{savingPackageEdit ? '…' : t('billing.admin.savePackage')}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Adjust Single Wallet Dialog */}
      <Dialog open={adjustingWallet !== null} onOpenChange={(open) => !open && setAdjustingWallet(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('billing.admin.addCredits')}</DialogTitle>
            <DialogDescription>
              {adjustingWallet && (
                <span>
                  Grant credit adjustment to{' '}
                  <strong className="text-foreground">{getUserDetails(adjustingWallet.user_id).name}</strong> (#{adjustingWallet.user_id})
                </span>
              )}
            </DialogDescription>
          </DialogHeader>
          <form className="space-y-4" onSubmit={saveCreditAdjustment}>
            <Field label={t('billing.admin.userId')} htmlFor="adjust-user-id">
              <Input id="adjust-user-id" type="number" value={adjustingWallet?.user_id ?? 0} disabled />
            </Field>
            <NumberField id="adjust-credits" label={t('billing.admin.creditsAmount')} value={creditAdjustmentForm.credits} onChange={(credits) => setCreditAdjustmentForm((current) => ({ ...current, credits }))} min={1} required />
            <Field label={t('billing.admin.reason')} htmlFor="adjust-reason">
              <Textarea id="adjust-reason" value={creditAdjustmentForm.reason} onChange={(event) => setCreditAdjustmentForm((current) => ({ ...current, reason: event.target.value }))} required placeholder="Explain why credits are being granted..." />
            </Field>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setAdjustingWallet(null)}>{t('common.cancel')}</Button>
              <Button type="submit" disabled={savingCreditAdjustment}>{savingCreditAdjustment ? '…' : t('billing.admin.saveCredits')}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Invoice Detail Modal */}
      <Dialog open={viewingInvoice !== null} onOpenChange={(open) => !open && setViewingInvoice(null)}>
        <DialogContent className="max-w-lg p-0 overflow-hidden">
          {viewingInvoice && (() => {
            const userDetails = getUserDetails(viewingInvoice.user_id)
            return (
              <div>
                <DialogHeader className="p-6 pb-4 border-b border-border/60">
                  <div className="flex items-center justify-between gap-3">
                    <DialogTitle className="text-base font-semibold font-mono">
                      #INV-{viewingInvoice.id}
                    </DialogTitle>
                    <Badge variant={statusVariant(viewingInvoice.status)} className="text-xs">
                      {formatStatus(viewingInvoice.status)}
                    </Badge>
                  </div>
                  <DialogDescription className="text-xs text-muted-foreground mt-1">
                    {t('billing.periodLabel')}: {formatDate(viewingInvoice.period_start)} &ndash; {formatDate(viewingInvoice.period_end)}
                  </DialogDescription>
                </DialogHeader>

                <div className="p-6 space-y-4 text-xs">
                  {/* Account detail */}
                  <div className="flex items-center justify-between rounded-lg border border-border/50 bg-muted/20 p-3">
                    <div>
                      <span className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">User Account</span>
                      <div className="font-semibold text-sm mt-0.5">{userDetails.name}</div>
                      <div className="text-muted-foreground">{userDetails.email || `#${viewingInvoice.user_id}`}</div>
                    </div>
                    <Badge variant="outline" className="font-mono text-xs">ID #{viewingInvoice.user_id}</Badge>
                  </div>

                  {/* Line items */}
                  <div>
                    <span className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground block mb-2">Line Items</span>
                    {viewingInvoice.items && viewingInvoice.items.length > 0 ? (
                      <div className="rounded-lg border border-border/50 divide-y divide-border/40 overflow-hidden">
                        {viewingInvoice.items.map((item) => (
                          <div key={item.id} className="p-3 flex items-start justify-between gap-3 bg-card/40">
                            <div>
                              <div className="font-semibold text-xs text-foreground flex items-center gap-1.5">
                                <span>{item.resource_name || item.description || 'Resource Usage'}</span>
                                <Badge variant="outline" className="text-[9px] capitalize py-0 px-1">{item.resource_type}</Badge>
                              </div>
                              <div className="text-[11px] text-muted-foreground mt-0.5">{item.spec_name}</div>
                            </div>
                            <div className="font-semibold tabular-nums text-xs whitespace-nowrap">
                              {formatCredits(item.credits)} {t('billing.credits')}
                            </div>
                          </div>
                        ))}
                      </div>
                    ) : (
                      <div className="p-4 text-center text-muted-foreground border border-dashed rounded-lg">
                        {t('billing.noMatchingInvoices')}
                      </div>
                    )}
                  </div>

                  {/* Summary */}
                  <div className="border-t border-border/60 pt-3 flex items-center justify-between font-semibold text-sm">
                    <span>Total Charged</span>
                    <span className="tabular-nums text-primary">
                      {formatCredits(viewingInvoice.total_credits)} {t('billing.credits')}
                    </span>
                  </div>
                </div>
              </div>
            )
          })()}
        </DialogContent>
      </Dialog>

      {/* Direct Add Credits Modal Triggered from Header */}
      <Dialog open={isDirectCreditModalOpen} onOpenChange={setIsDirectCreditModalOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('billing.admin.addCreditsForUser')}</DialogTitle>
            <DialogDescription>{t('billing.admin.addCreditsForUserDescription')}</DialogDescription>
          </DialogHeader>
          <form className="space-y-4" onSubmit={saveDirectCreditAdjustment}>
            <Field label={t('billing.admin.selectUser')} htmlFor="modal-direct-credits-user-id">
              <Select
                value={directCreditAdjustmentForm.user_id ? String(directCreditAdjustmentForm.user_id) : ''}
                onValueChange={(val) => setDirectCreditAdjustmentForm((current) => ({ ...current, user_id: Number(val) }))}
              >
                <SelectTrigger id="modal-direct-credits-user-id" className="w-full">
                  {usersList.find((u) => u.id === directCreditAdjustmentForm.user_id)
                    ? `${usersList.find((u) => u.id === directCreditAdjustmentForm.user_id)?.name} (${usersList.find((u) => u.id === directCreditAdjustmentForm.user_id)?.email})`
                    : directCreditAdjustmentForm.user_id
                    ? `${t('billing.admin.user')} #${directCreditAdjustmentForm.user_id}`
                    : t('billing.admin.chooseUser')}
                </SelectTrigger>
                <SelectContent>
                  {usersList.map((user) => (
                    <SelectItem key={user.id} value={String(user.id)}>
                      {user.name} ({user.email}) · <span className="capitalize">{user.role}</span>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <NumberField id="modal-direct-credits-manual-user-id" label={t('billing.admin.userId')} value={directCreditAdjustmentForm.user_id} onChange={(user_id) => setDirectCreditAdjustmentForm((current) => ({ ...current, user_id }))} min={1} required />
            <NumberField id="modal-direct-credits-amount" label={t('billing.admin.creditsAmount')} value={directCreditAdjustmentForm.credits} onChange={(credits) => setDirectCreditAdjustmentForm((current) => ({ ...current, credits }))} min={1} required />
            <Field label={t('billing.admin.reason')} htmlFor="modal-direct-credits-reason">
              <Textarea id="modal-direct-credits-reason" value={directCreditAdjustmentForm.reason} onChange={(event) => setDirectCreditAdjustmentForm((current) => ({ ...current, reason: event.target.value }))} required placeholder="Audit reason for granting credits..." />
            </Field>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setIsDirectCreditModalOpen(false)}>{t('common.cancel')}</Button>
              <Button type="submit" disabled={savingDirectCreditAdjustment}>{savingDirectCreditAdjustment ? '…' : t('billing.admin.saveCredits')}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function PricingForm({
  title,
  description,
  children,
  onSubmit,
  submitting,
  submitLabel,
  secondaryAction,
}: {
  title: string
  description: string
  children: React.ReactNode
  onSubmit: (event: React.FormEvent<HTMLFormElement>) => void
  submitting: boolean
  submitLabel: string
  secondaryAction?: React.ReactNode
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base font-semibold">{title}</CardTitle>
        <CardDescription className="text-xs">{description}</CardDescription>
      </CardHeader>
      <CardContent>
        <form className="space-y-4" onSubmit={onSubmit}>
          {children}
          <div className="flex items-center gap-2 pt-2">
            <Button type="submit" disabled={submitting}>
              {submitting ? '…' : submitLabel}
            </Button>
            {secondaryAction}
          </div>
        </form>
      </CardContent>
    </Card>
  )
}

function Field({ label, htmlFor, hint, children }: { label: string; htmlFor?: string; hint?: string; children: React.ReactNode }) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={htmlFor}>{label}</Label>
      {children}
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  )
}

function TextField({
  label,
  id,
  value,
  onChange,
  required,
  pattern,
  placeholder,
  hint,
  disabled,
}: {
  label: string
  id?: string
  value: string
  onChange: (value: string) => void
  required?: boolean
  pattern?: string
  placeholder?: string
  hint?: string
  disabled?: boolean
}) {
  return (
    <Field label={label} htmlFor={id} hint={hint}>
      <Input id={id} value={value} onChange={(event) => onChange(event.target.value)} required={required} pattern={pattern} placeholder={placeholder} disabled={disabled} />
    </Field>
  )
}

function NumberField({
  label,
  id,
  value,
  onChange,
  min,
  placeholder,
  required,
  hint,
}: {
  label: string
  id?: string
  value: number | ''
  onChange: (value: number | '') => void
  min?: number
  placeholder?: string
  required?: boolean
  hint?: string
}) {
  return (
    <Field label={label} htmlFor={id} hint={hint}>
      <Input id={id} type="number" min={min} value={value} onChange={(event) => onChange(event.target.value === '' ? '' : Number(event.target.value))} required={required} placeholder={placeholder} />
    </Field>
  )
}
