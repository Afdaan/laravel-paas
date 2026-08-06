import { useCallback, useEffect, useState } from 'react'
import { AlertTriangle, CreditCard, Pencil, Plus, ReceiptText, RefreshCw, WalletCards } from 'lucide-react'
import { toast } from 'sonner'
import { billingAPI, usersAPI } from '@/services/api'
import { User } from '@/types'
import { createAdjustmentIdempotencyKey } from '@/lib/billing-ui'
import useAuthStore from '@/stores/authStore'
import useTranslation from '@/lib/useTranslation'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'

type Catalog = {
  specs: Array<{ id: number; name: string; slug: string; type: 'project' | 'database'; version: number; monthly_credits: number; cpu_millicores: number; memory_mb: number; storage_gb: number; connection_limit?: number; backup_retention_days?: number; is_active: boolean }>
  packages: Array<{ id: number; credits: number; amount_minor: number; currency: string; sort_order: number; version: number; is_active: boolean }>
}
type Suspension = { user_id: number; resource_id: number; resource_type: string; status: string; oldest_due_at?: string; payment_due_days: number }
type Page<T> = { data: T[]; page: number; limit: number; total: number }
type Wallet = { user_id: number; balance_credits: number; updated_at: string }
type Invoice = { id: number; user_id: number; total_credits: number; status: string; period_start: string; period_end: string; due_at?: string; paid_at?: string }
type Topup = { id: number; user_id: number; credits: number; amount_minor: number; currency: string; status: string; created_at: string; paid_at?: string }
type TopupPackage = { id: number; credits: number; amount_minor: number; currency: string; sort_order: number }
type SpecForm = {
  type: 'project' | 'database'
  name: string
  slug: string
  cpu_millicores: number
  memory_mb: number
  storage_gb: number
  monthly_credits: number
  connection_limit: number | ''
  backup_retention_days: number | ''
  reason: string
}

const PAGE_LIMIT = 20
const emptySpec: SpecForm = { type: 'project', name: '', slug: '', cpu_millicores: 500, memory_mb: 512, storage_gb: 1, monthly_credits: 0, connection_limit: '', backup_retention_days: '', reason: '' }
const emptyPackage = { credits: 0, amount_minor: 0, sort_order: 0, reason: '' }
const emptyPackageEdit = { amount_minor: 0, sort_order: 0, reason: '' }
const emptyCreditAdjustment = { credits: 0, reason: '' }
const emptyDirectCreditAdjustment = { user_id: 0, credits: 0, reason: '' }
const statusVariant = (status: string) => status === 'paid' || status === 'active' ? 'secondary' : status === 'suspended' || status === 'payment_due' ? 'destructive' : 'outline'

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
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [loading, setLoading] = useState(true)
  const [specForm, setSpecForm] = useState(emptySpec)
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

  const locale = language === 'id' ? 'id-ID' : 'en-US'
  const formatCredits = useCallback((value: number) => new Intl.NumberFormat(locale).format(value), [locale])
  const formatDate = useCallback((value?: string) => value ? new Intl.DateTimeFormat(locale, { dateStyle: 'medium' }).format(new Date(value)) : '—', [locale])
  const formatMoney = useCallback((amountMinor: number, currency: string) => new Intl.NumberFormat(locale, { style: 'currency', currency, maximumFractionDigits: 0 }).format(amountMinor), [locale])
  const formatStatus = useCallback((status: string) => {
    const translated = t(`billing.statuses.${status}`)
    if (translated !== `billing.statuses.${status}`) return translated
    const topup = t(`billing.topupStatuses.${status}`)
    if (topup !== `billing.topupStatuses.${status}`) return topup
    return status.replace(/_/g, ' ')
  }, [t])

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
        billingAPI.adminWallets({ page: walletPage, limit: PAGE_LIMIT }),
        billingAPI.adminInvoices({ page: invoicePage, limit: PAGE_LIMIT }),
        billingAPI.adminTopups({ page: topupPage, limit: PAGE_LIMIT }),
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
  }, [invoicePage, t, topupPage, walletPage])

  useEffect(() => {
    void load()
  }, [load])

  const getErrorMessage = (err: unknown, fallback: string) => {
    return (err as { response?: { data?: { message?: string } } })?.response?.data?.message || fallback
  }

  const createSpec = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!isSuperAdmin) return
    setCreatingSpec(true)
    try {
      const { connection_limit, backup_retention_days, ...spec } = specForm
      await billingAPI.createSpec(
        specForm.type === 'database'
          ? { ...spec, connection_limit: Number(connection_limit), backup_retention_days: Number(backup_retention_days) }
          : spec,
      )
      setSpecForm(emptySpec)
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
      await billingAPI.createTopupPackage(packageForm)
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
    setPackageEditForm({ amount_minor: pkg.amount_minor, sort_order: pkg.sort_order, reason: '' })
  }

  const savePackageEdit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!isSuperAdmin || !editingPackage) return
    setSavingPackageEdit(true)
    try {
      await billingAPI.updateTopupPackage(editingPackage.id, { ...packageEditForm, credits: editingPackage.credits })
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

  const startAdjustWallet = (wallet: Wallet) => {
    setAdjustingWallet(wallet)
    setCreditAdjustmentForm(emptyCreditAdjustment)
  }

  const saveCreditAdjustment = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!isSuperAdmin || !adjustingWallet) return
    setSavingCreditAdjustment(true)
    try {
      await billingAPI.adjustWalletCredits(adjustingWallet.user_id, creditAdjustmentForm, createAdjustmentIdempotencyKey())
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
    try {
      const { user_id, ...payload } = directCreditAdjustmentForm
      await billingAPI.adjustWalletCredits(user_id, payload, createAdjustmentIdempotencyKey())
      setDirectCreditAdjustmentForm(emptyDirectCreditAdjustment)
      toast.success(t('billing.admin.adjustmentSuccess'))
      await load()
    } catch (err) {
      toast.error(getErrorMessage(err, t('billing.admin.adjustmentFailed')))
    } finally {
      setSavingDirectCreditAdjustment(false)
    }
  }

  return <div className="mx-auto max-w-7xl space-y-6 pb-10">
    <div className="flex flex-col gap-3 border-b pb-5 sm:flex-row sm:items-end sm:justify-between">
      <div><p className="text-sm font-medium text-primary">{t('common.adminPanel')}</p><h1 className="text-2xl font-semibold tracking-tight">{t('billing.admin.title')}</h1><p className="mt-1 text-sm text-muted-foreground">{t('billing.admin.description')}</p></div>
      <Button variant="outline" size="sm" onClick={() => void load()} disabled={loading}><RefreshCw className={loading ? 'animate-spin' : ''} /> {t('billing.refresh')}</Button>
    </div>

    {Object.keys(errors).length > 0 && <Card className="border-amber-500/40 bg-amber-500/5" role="status"><CardContent className="pt-6 text-sm">{t('billing.loadPartial', { sections: Object.values(errors).join(', ') })}</CardContent></Card>}

    <div className="grid gap-6 xl:grid-cols-2">
      <CatalogCard title={t('billing.admin.resourcePlans')} rows={catalog?.specs.map((spec) => ({ id: spec.id, title: `${spec.name} · ${formatCredits(spec.monthly_credits)} ${t('billing.credits')}`, detail: `${t(`billing.resourceTypes.${spec.type}`)} · v${spec.version} · ${spec.cpu_millicores}m CPU · ${spec.memory_mb} MB${spec.connection_limit ? ` · ${t('billing.connections', { count: spec.connection_limit })}` : ''}`, active: spec.is_active }))} loading={loading} error={errors.plans} onRetry={() => void load()} empty={t('billing.admin.noRecords')} />
      <CatalogCard
        title={t('billing.admin.topupPackages')}
        rows={catalog?.packages.map((pkg) => ({ id: pkg.id, title: `${formatCredits(pkg.credits)} ${t('billing.credits')} · ${formatMoney(pkg.amount_minor, pkg.currency)}`, detail: `v${pkg.version}`, active: pkg.is_active }))}
        loading={loading}
        error={errors.plans}
        onRetry={() => void load()}
        empty={t('billing.admin.noRecords')}
        onEdit={isSuperAdmin ? (id) => {
          const pkg = catalog?.packages.find((p) => p.id === id)
          if (pkg) startEditPackage(pkg)
        } : undefined}
      />
    </div>

    {isSuperAdmin ? <section className="grid gap-6 xl:grid-cols-3">
      <PricingForm title={t('billing.admin.createPlan')} description={t('billing.admin.versionedPricing')} onSubmit={createSpec} submitting={creatingSpec} submitLabel={t('billing.admin.create')}>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label={t('billing.admin.resourceType')} htmlFor="spec-type"><Select name="type" value={specForm.type} onValueChange={(type) => setSpecForm((current) => ({ ...current, type: type as 'project' | 'database', connection_limit: '', backup_retention_days: '' }))}><SelectTrigger id="spec-type" className="w-full">{specForm.type === 'project' ? t('billing.admin.project') : t('billing.admin.database')}</SelectTrigger><SelectContent><SelectItem value="project">{t('billing.admin.project')}</SelectItem><SelectItem value="database">{t('billing.admin.database')}</SelectItem></SelectContent></Select></Field>
          <NumberField id="spec-monthly-credits" label={t('billing.admin.monthlyCredits')} value={specForm.monthly_credits} onChange={(monthly_credits) => setSpecForm((current) => ({ ...current, monthly_credits }))} min={1} />
          <TextField id="spec-name" label={t('billing.admin.name')} value={specForm.name} onChange={(name) => setSpecForm((current) => ({ ...current, name }))} required />
          <TextField id="spec-slug" label={t('billing.admin.slug')} value={specForm.slug} onChange={(slug) => setSpecForm((current) => ({ ...current, slug }))} pattern="[a-z0-9-]+" required />
          {specForm.type === 'project' ? (
            <>
              <NumberField id="spec-cpu" label={t('billing.admin.cpu')} value={specForm.cpu_millicores} onChange={(cpu_millicores) => setSpecForm((current) => ({ ...current, cpu_millicores }))} min={1} />
              <NumberField id="spec-memory" label={t('billing.admin.memory')} value={specForm.memory_mb} onChange={(memory_mb) => setSpecForm((current) => ({ ...current, memory_mb }))} min={1} />
              <NumberField id="spec-storage" label={t('billing.admin.storage')} value={specForm.storage_gb} onChange={(storage_gb) => setSpecForm((current) => ({ ...current, storage_gb }))} min={1} />
            </>
          ) : (
            <>
              <Field label={t('billing.admin.connectionLimit')} htmlFor="spec-connection-limit">
                <Input id="spec-connection-limit" type="number" min={1} max={1000000} value={specForm.connection_limit} onChange={(event) => setSpecForm((current) => ({ ...current, connection_limit: event.target.value === '' ? '' : Number(event.target.value) }))} required />
              </Field>
              <Field label={t('billing.admin.backupRetentionDays')} htmlFor="spec-backup-retention">
                <Input id="spec-backup-retention" type="number" min={1} max={3650} value={specForm.backup_retention_days} onChange={(event) => setSpecForm((current) => ({ ...current, backup_retention_days: event.target.value === '' ? '' : Number(event.target.value) }))} required />
              </Field>
            </>
          )}
        </div>
        <Field label={t('billing.admin.reason')} htmlFor="spec-reason"><Textarea id="spec-reason" value={specForm.reason} onChange={(event) => setSpecForm((current) => ({ ...current, reason: event.target.value }))} required /></Field>
      </PricingForm>
      <PricingForm title={t('billing.admin.createPackage')} description={t('billing.admin.versionedPricing')} onSubmit={createPackage} submitting={creatingPackage} submitLabel={t('billing.admin.create')}>
        <div className="grid gap-3 sm:grid-cols-2"><NumberField id="package-credits" label={t('billing.admin.credits')} value={packageForm.credits} onChange={(credits) => setPackageForm((current) => ({ ...current, credits }))} min={1} /><NumberField id="package-amount" label={t('billing.admin.amountMinor')} value={packageForm.amount_minor} onChange={(amount_minor) => setPackageForm((current) => ({ ...current, amount_minor }))} min={1} /><NumberField id="package-sort-order" label={t('billing.admin.sortOrder')} value={packageForm.sort_order} onChange={(sort_order) => setPackageForm((current) => ({ ...current, sort_order }))} min={0} /></div>
        <Field label={t('billing.admin.reason')} htmlFor="package-reason"><Textarea id="package-reason" value={packageForm.reason} onChange={(event) => setPackageForm((current) => ({ ...current, reason: event.target.value }))} required /></Field>
      </PricingForm>
      <PricingForm title={t('billing.admin.addCreditsForUser')} description={t('billing.admin.addCreditsForUserDescription')} onSubmit={saveDirectCreditAdjustment} submitting={savingDirectCreditAdjustment} submitLabel={t('billing.admin.saveCredits')}>
        <Field label={t('billing.admin.selectUser')} htmlFor="direct-credits-user-id">
          <Select
            value={directCreditAdjustmentForm.user_id ? String(directCreditAdjustmentForm.user_id) : ''}
            onValueChange={(val) => setDirectCreditAdjustmentForm((current) => ({ ...current, user_id: Number(val) }))}
          >
            <SelectTrigger id="direct-credits-user-id" className="w-full">
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
        <NumberField id="direct-credits-manual-user-id" label={t('billing.admin.userId')} value={directCreditAdjustmentForm.user_id} onChange={(user_id) => setDirectCreditAdjustmentForm((current) => ({ ...current, user_id }))} min={1} />
        <NumberField id="direct-credits-amount" label={t('billing.admin.creditsAmount')} value={directCreditAdjustmentForm.credits} onChange={(credits) => setDirectCreditAdjustmentForm((current) => ({ ...current, credits }))} min={1} />
        <Field label={t('billing.admin.reason')} htmlFor="direct-credits-reason"><Textarea id="direct-credits-reason" value={directCreditAdjustmentForm.reason} onChange={(event) => setDirectCreditAdjustmentForm((current) => ({ ...current, reason: event.target.value }))} required /></Field>
      </PricingForm>
    </section> : <Card><CardContent className="pt-6 text-sm text-muted-foreground">{t('billing.admin.superadminOnly')}</CardContent></Card>}

    <section className="grid gap-6 xl:grid-cols-3">
      <CollectionCard title={t('billing.admin.wallets')} icon={<WalletCards className="size-4" />} page={wallets} loading={loading} error={errors.wallets} onRetry={() => void load()} empty={t('billing.admin.noRecords')} meta={(wallet) => `${t('billing.admin.user')} #${wallet.user_id} · ${formatDate(wallet.updated_at)}`} value={(wallet) => `${formatCredits(wallet.balance_credits)} ${t('billing.credits')}`} onPageChange={setWalletPage} onAdjust={isSuperAdmin ? startAdjustWallet : undefined} />
      <CollectionCard title={t('billing.invoices')} icon={<ReceiptText className="size-4" />} page={invoices} loading={loading} error={errors.invoices} onRetry={() => void load()} empty={t('billing.admin.noRecords')} meta={(invoice) => `${t('billing.admin.user')} #${invoice.user_id} · ${formatDate(invoice.period_start)} – ${formatDate(invoice.period_end)}`} value={(invoice) => `${formatCredits(invoice.total_credits)} ${t('billing.credits')}`} status={(invoice) => invoice.status} formatStatus={formatStatus} onPageChange={setInvoicePage} />
      <CollectionCard title={t('billing.topups')} icon={<CreditCard className="size-4" />} page={topups} loading={loading} error={errors.topups} onRetry={() => void load()} empty={t('billing.admin.noRecords')} meta={(topup) => `${t('billing.admin.user')} #${topup.user_id} · ${formatDate(topup.paid_at ?? topup.created_at)}`} value={(topup) => `${formatCredits(topup.credits)} ${t('billing.credits')} · ${formatMoney(topup.amount_minor, topup.currency)}`} status={(topup) => topup.status} formatStatus={formatStatus} onPageChange={setTopupPage} />
    </section>

    <Card><CardHeader><CardTitle className="flex items-center gap-2 text-base"><AlertTriangle className="size-4 text-destructive" />{t('billing.admin.suspensions')}</CardTitle></CardHeader><CardContent className="space-y-3">{suspensions?.map((item) => <div key={`${item.user_id}-${item.resource_type}-${item.resource_id}`} className="flex items-center justify-between gap-3 border-b pb-3 last:border-0"><div><p className="text-sm font-medium">{t('billing.admin.user')} #{item.user_id} · {t(`billing.resourceTypes.${item.resource_type}`) !== `billing.resourceTypes.${item.resource_type}` ? t(`billing.resourceTypes.${item.resource_type}`) : item.resource_type} #{item.resource_id}</p><p className="text-xs text-muted-foreground">{item.oldest_due_at ? `${formatDate(item.oldest_due_at)} · ` : ''}{t('billing.days', { count: item.payment_due_days })}</p></div><Badge variant={statusVariant(item.status)}>{formatStatus(item.status)}</Badge></div>)}{suspensions !== null && suspensions.length === 0 && <p className="text-sm text-muted-foreground">{t('billing.admin.noRecords')}</p>}</CardContent></Card>

    <Dialog open={editingPackage !== null} onOpenChange={(open) => !open && setEditingPackage(null)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('billing.admin.editPackage')}</DialogTitle>
          <DialogDescription>{t('billing.admin.editPackageDescription')}</DialogDescription>
        </DialogHeader>
        <form className="space-y-4" onSubmit={savePackageEdit}>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label={t('billing.admin.credits')} htmlFor="edit-package-credits"><Input id="edit-package-credits" type="number" value={editingPackage?.credits ?? 0} disabled /></Field>
            <NumberField id="edit-package-amount" label={t('billing.admin.amountMinor')} value={packageEditForm.amount_minor} onChange={(amount_minor) => setPackageEditForm((current) => ({ ...current, amount_minor }))} min={1} />
            <NumberField id="edit-package-sort-order" label={t('billing.admin.sortOrder')} value={packageEditForm.sort_order} onChange={(sort_order) => setPackageEditForm((current) => ({ ...current, sort_order }))} min={0} />
          </div>
          <Field label={t('billing.admin.reason')} htmlFor="edit-package-reason"><Textarea id="edit-package-reason" value={packageEditForm.reason} onChange={(event) => setPackageEditForm((current) => ({ ...current, reason: event.target.value }))} required /></Field>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setEditingPackage(null)}>{t('common.cancel')}</Button>
            <Button type="submit" disabled={savingPackageEdit}>{savingPackageEdit ? '…' : t('billing.admin.savePackage')}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>

    <Dialog open={adjustingWallet !== null} onOpenChange={(open) => !open && setAdjustingWallet(null)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('billing.admin.addCredits')}</DialogTitle>
          <DialogDescription>{t('billing.admin.addCreditsDescription')}</DialogDescription>
        </DialogHeader>
        <form className="space-y-4" onSubmit={saveCreditAdjustment}>
          <Field label={t('billing.admin.userId')} htmlFor="adjust-user-id"><Input id="adjust-user-id" type="number" value={adjustingWallet?.user_id ?? 0} disabled /></Field>
          <NumberField id="adjust-credits" label={t('billing.admin.creditsAmount')} value={creditAdjustmentForm.credits} onChange={(credits) => setCreditAdjustmentForm((current) => ({ ...current, credits }))} min={1} />
          <Field label={t('billing.admin.reason')} htmlFor="adjust-reason"><Textarea id="adjust-reason" value={creditAdjustmentForm.reason} onChange={(event) => setCreditAdjustmentForm((current) => ({ ...current, reason: event.target.value }))} required /></Field>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setAdjustingWallet(null)}>{t('common.cancel')}</Button>
            <Button type="submit" disabled={savingCreditAdjustment}>{savingCreditAdjustment ? '…' : t('billing.admin.saveCredits')}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  </div>
}

function CatalogCard({ title, rows, loading, error, onRetry, empty, onEdit }: { title: string; rows?: Array<{ id: number; title: string; detail: string; active: boolean }>; loading: boolean; error?: string; onRetry?: () => void; empty: string; onEdit?: (id: number) => void }) {
  const { t } = useTranslation()
  return <Card><CardHeader><CardTitle className="text-base">{title}</CardTitle></CardHeader><CardContent className="space-y-3">{loading && Array.from({ length: 3 }).map((_, i) => <Skeleton key={i} className="h-14 w-full" />)}{!loading && error && !rows && <div className="space-y-2"><p className="text-sm text-destructive">{t('billing.loadError', { section: title })}</p>{onRetry && <Button size="sm" variant="outline" onClick={onRetry}>{t('billing.retry')}</Button>}</div>}{!loading && rows?.map((row) => <div key={row.id} className="flex items-center justify-between gap-3 border-b pb-3 last:border-0"><div><p className="text-sm font-medium">{row.title}</p><p className="text-xs text-muted-foreground">{row.detail}</p></div><div className="flex items-center gap-2">{onEdit && row.active && <Button size="icon-xs" variant="ghost" onClick={() => onEdit(row.id)} aria-label={t('billing.admin.editPackage')}><Pencil className="size-3.5" /></Button>}<Badge variant={row.active ? 'secondary' : 'outline'}>{row.active ? t('billing.admin.active') : t('billing.admin.inactive')}</Badge></div></div>)}{!loading && rows?.length === 0 && <p className="text-sm text-muted-foreground">{empty}</p>}</CardContent></Card>
}

function CollectionCard<T extends { id?: number; user_id: number }>({ title, icon, page, loading, error, onRetry, empty, meta, value, status, formatStatus = (status) => status.replace('_', ' '), onPageChange, onAdjust }: { title: string; icon: React.ReactNode; page: Page<T> | null; loading: boolean; error?: string; onRetry?: () => void; empty: string; meta: (row: T) => string; value: (row: T) => string; status?: (row: T) => string; formatStatus?: (status: string) => string; onPageChange: (page: number) => void; onAdjust?: (row: T) => void }) {
  const { t } = useTranslation()
  const hasPrevious = (page?.page ?? 1) > 1
  const hasNext = page ? page.page * page.limit < page.total : false
  return <Card><CardHeader><CardTitle className="flex items-center gap-2 text-base">{icon}{title}</CardTitle><CardDescription>{page ? t('billing.admin.total', { count: page.data.length, total: page.total }) : ''}</CardDescription></CardHeader><CardContent className="space-y-3">{loading && Array.from({ length: 3 }).map((_, i) => <Skeleton key={i} className="h-14 w-full" />)}{!loading && error && !page && <div className="space-y-2"><p className="text-sm text-destructive">{t('billing.loadError', { section: title })}</p>{onRetry && <Button size="sm" variant="outline" onClick={onRetry}>{t('billing.retry')}</Button>}</div>}{!loading && page?.data.map((row, index) => <div key={row.id ?? `${row.user_id}-${index}`} className="flex items-center justify-between gap-3 border-b pb-3 last:border-0"><div className="min-w-0"><p className="text-sm font-medium">{value(row)}</p><p className="truncate text-xs text-muted-foreground">{meta(row)}</p></div><div className="flex items-center gap-2">{onAdjust && <Button size="icon-xs" variant="ghost" onClick={() => onAdjust(row)} aria-label={t('billing.admin.addCredits')}><Plus className="size-3.5" /></Button>}{status && <Badge variant={statusVariant(status(row))}>{formatStatus(status(row))}</Badge>}</div></div>)}{!loading && page?.data.length === 0 && <p className="text-sm text-muted-foreground">{empty}</p>}{page && <div className="flex justify-end gap-2 pt-1"><Button size="sm" variant="outline" disabled={!hasPrevious || loading} onClick={() => onPageChange(page.page - 1)}>{t('common.previous')}</Button><Button size="sm" variant="outline" disabled={!hasNext || loading} onClick={() => onPageChange(page.page + 1)}>{t('common.next')}</Button></div>}</CardContent></Card>
}

function PricingForm({ title, description, children, onSubmit, submitting, submitLabel }: { title: string; description: string; children: React.ReactNode; onSubmit: (event: React.FormEvent<HTMLFormElement>) => void; submitting: boolean; submitLabel: string }) {
  return <Card><CardHeader><CardTitle className="text-base">{title}</CardTitle><CardDescription>{description}</CardDescription></CardHeader><CardContent><form className="space-y-4" onSubmit={onSubmit}>{children}<Button type="submit" disabled={submitting}>{submitting ? '…' : submitLabel}</Button></form></CardContent></Card>
}

function Field({ label, htmlFor, children }: { label: string; htmlFor?: string; children: React.ReactNode }) { return <div className="space-y-1.5"><Label htmlFor={htmlFor}>{label}</Label>{children}</div> }
function TextField({ label, id, value, onChange, required, pattern }: { label: string; id?: string; value: string; onChange: (value: string) => void; required?: boolean; pattern?: string }) { return <Field label={label} htmlFor={id}><Input id={id} value={value} onChange={(event) => onChange(event.target.value)} required={required} pattern={pattern} /></Field> }
function NumberField({ label, id, value, onChange, min }: { label: string; id?: string; value: number; onChange: (value: number) => void; min: number }) { return <Field label={label} htmlFor={id}><Input id={id} type="number" min={min} value={value} onChange={(event) => onChange(event.target.value === '' ? 0 : Number(event.target.value))} required /></Field> }
