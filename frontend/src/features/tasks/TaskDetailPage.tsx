// 清淤任务详情：任务信息 + 可执行动作 + 清淤记录 + 验收结论。
import { useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { toErrorMessage } from '../../api/client';
import { recordApi } from '../../api/records';
import { taskApi } from '../../api/tasks';
import { ConfirmDialog } from '../../components/ConfirmDialog';
import { DataTable, type Column } from '../../components/DataTable';
import { InfoList } from '../../components/InfoList';
import { Modal } from '../../components/Modal';
import { PageHeader } from '../../components/PageHeader';
import { SectionCard } from '../../components/SectionCard';
import { StatCard } from '../../components/StatCard';
import { StatusTag } from '../../components/StatusTag';
import { StateBlock } from '../../components/StateBlock';
import { useToast } from '../../components/Toast';
import { useAsync } from '../../hooks/useAsync';
import type { RecordListItem, TaskAction, TeamReassignment, TeamWorkload } from '../../types/domain';
import {
  formatDate,
  formatDateTime,
  formatLength,
  formatNumber,
  formatVolume,
  today
} from '../../utils/format';

export function TaskDetailPage() {
  const params = useParams();
  const navigate = useNavigate();
  const toast = useToast();
  const id = Number(params.id ?? '0');

  const detail = useAsync(
    () => (id > 0 ? taskApi.detail(id) : Promise.reject(new Error('任务编号无效'))),
    [id]
  );
  const records = useAsync(
    () => (id > 0 ? recordApi.list({ taskId: id, pageSize: 50 }) : Promise.reject(new Error('任务编号无效'))),
    [id]
  );

  const [busy, setBusy] = useState(false);
  const [confirmAction, setConfirmAction] = useState<'complete' | 'delete' | null>(null);
  const [cancelOpen, setCancelOpen] = useState(false);
  const [cancelReason, setCancelReason] = useState('');
  const [cancelError, setCancelError] = useState('');

  const [reassignOpen, setReassignOpen] = useState(false);
  const [toTeam, setToTeam] = useState('');
  const [reassignReason, setReassignReason] = useState('');
  const [effectiveDate, setEffectiveDate] = useState('');
  const [operator, setOperator] = useState('');
  const [reassignError, setReassignError] = useState('');

  const task = detail.data?.task;
  const totals = detail.data?.recordTotals;
  const acceptance = detail.data?.acceptance;
  const allowed: TaskAction[] = detail.data?.allowedActions ?? [];
  const can = (action: TaskAction) => allowed.includes(action);

  const runAction = async (message: string, action: () => Promise<unknown>) => {
    setBusy(true);
    try {
      await action();
      toast.success(message);
      setConfirmAction(null);
      detail.reload();
      records.reload();
    } catch (cause: unknown) {
      toast.error(toErrorMessage(cause));
    } finally {
      setBusy(false);
    }
  };

  const handleDelete = async () => {
    setBusy(true);
    try {
      await taskApi.remove(id);
      toast.success('清淤任务已删除');
      navigate('/tasks');
    } catch (cause: unknown) {
      toast.error(toErrorMessage(cause));
      setConfirmAction(null);
    } finally {
      setBusy(false);
    }
  };

  const submitCancel = async () => {
    if (!cancelReason.trim()) {
      setCancelError('请填写取消原因');
      return;
    }
    setCancelError('');
    await runAction('任务已取消', () => taskApi.cancel(id, cancelReason.trim()));
    setCancelOpen(false);
    setCancelReason('');
  };

  const openReassign = () => {
    setToTeam('');
    setReassignReason('');
    setEffectiveDate(today());
    setOperator('');
    setReassignError('');
    setReassignOpen(true);
  };

  const submitReassign = async () => {
    const nextTeam = toTeam.trim();
    const reason = reassignReason.trim();
    if (!nextTeam) {
      setReassignError('请填写接手班组');
      return;
    }
    if (task && nextTeam === task.teamName) {
      setReassignError('接手班组与当前班组相同，无需改派');
      return;
    }
    if (!reason) {
      setReassignError('请填写改派原因');
      return;
    }
    if (!effectiveDate) {
      setReassignError('请选择生效日期');
      return;
    }
    setReassignError('');
    setBusy(true);
    try {
      await taskApi.reassign(id, {
        toTeamName: nextTeam,
        reason,
        effectiveDate,
        operatorName: operator.trim() || undefined
      });
      toast.success(`任务已改派给 ${nextTeam}`);
      setReassignOpen(false);
      detail.reload();
      records.reload();
    } catch (cause: unknown) {
      toast.error(toErrorMessage(cause));
    } finally {
      setBusy(false);
    }
  };

  const teamWorkload: TeamWorkload[] = detail.data?.teamWorkload ?? [];
  const reassignments: TeamReassignment[] = detail.data?.reassignments ?? [];

  const recordColumns: Column<RecordListItem>[] = [
    {
      key: 'code',
      title: '记录编号',
      width: '170px',
      render: (row) => (
        <Link className="cell-main" to={`/records/${row.id}`}>
          {row.code}
        </Link>
      )
    },
    { key: 'cleanedAt', title: '清淤日期', width: '110px', render: (row) => formatDate(row.cleanedAt) },
    { key: 'method', title: '清淤方式', width: '120px', render: (row) => <StatusTag list="cleaningMethods" value={row.method} /> },
    { key: 'weather', title: '天气', width: '90px', render: (row) => <StatusTag list="weathers" value={row.weather} /> },
    { key: 'lengthM', title: '清淤长度', width: '110px', align: 'right', render: (row) => formatLength(row.lengthM) },
    {
      key: 'sludgeVolumeM3',
      title: '清淤量',
      width: '110px',
      align: 'right',
      render: (row) => formatVolume(row.sludgeVolumeM3)
    },
    { key: 'personnelCount', title: '作业人数', width: '90px', align: 'right', render: (row) => formatNumber(row.personnelCount, 0) },
    { key: 'recorderName', title: '记录人', width: '100px', render: (row) => row.recorderName || '—' }
  ];

  const teamColumns: Column<TeamWorkload>[] = [
    { key: 'teamName', title: '归属班组', render: (row) => <span className="cell-main">{row.teamName}</span> },
    { key: 'recordCount', title: '记录条数', align: 'right', render: (row) => formatNumber(row.recordCount, 0) },
    { key: 'personDays', title: '作业工日', align: 'right', render: (row) => formatNumber(row.personDays, 0) },
    { key: 'sludgeVolumeM3', title: '清淤量', align: 'right', render: (row) => formatVolume(row.sludgeVolumeM3) },
    { key: 'cleanedLengthM', title: '清淤长度', align: 'right', render: (row) => formatLength(row.cleanedLengthM) }
  ];

  const reassignmentColumns: Column<TeamReassignment>[] = [
    { key: 'effectiveDate', title: '生效日期', width: '110px', render: (row) => formatDate(row.effectiveDate) },
    {
      key: 'teams',
      title: '原班组 → 接手班组',
      render: (row) => (
        <span>
          <span className="tag tag-muted">{row.fromTeamName}</span>
          <span style={{ margin: '0 8px' }}>→</span>
          <span className="tag tag-primary">{row.toTeamName}</span>
        </span>
      )
    },
    { key: 'reason', title: '改派原因', render: (row) => row.reason },
    { key: 'operatorName', title: '操作人', width: '100px', render: (row) => row.operatorName || '—' },
    { key: 'createdAt', title: '登记时间', width: '150px', render: (row) => formatDateTime(row.createdAt) }
  ];

  return (
    <div className="page">
      <PageHeader
        title={task ? `${task.code} ${task.title}` : '清淤任务详情'}
        description="任务状态驱动清淤记录录入与验收登记：待开工 → 清淤中 → 待验收 → 已验收。"
        extra={task ? <StatusTag list="taskStatuses" value={task.status} /> : null}
        actions={
          <>
            <button type="button" className="btn btn-ghost" onClick={() => navigate('/tasks')}>
              返回列表
            </button>
            {can('edit') ? (
              <button type="button" className="btn btn-ghost" onClick={() => navigate(`/tasks/${id}/edit`)}>
                编辑
              </button>
            ) : null}
            {can('start') ? (
              <button type="button" className="btn btn-primary" disabled={busy} onClick={() => void runAction('任务已开工', () => taskApi.start(id))}>
                开工
              </button>
            ) : null}
            {can('complete') ? (
              <button type="button" className="btn btn-primary" disabled={busy} onClick={() => setConfirmAction('complete')}>
                完工报验
              </button>
            ) : null}
            {can('accept') ? (
              <button
                type="button"
                className="btn btn-primary"
                onClick={() => navigate(`/acceptances/new?taskId=${id}`)}
              >
                登记验收
              </button>
            ) : null}
            {can('cancel') ? (
              <button type="button" className="btn btn-ghost" onClick={() => setCancelOpen(true)}>
                取消任务
              </button>
            ) : null}
            {can('reassign') ? (
              <button type="button" className="btn btn-primary" disabled={busy} onClick={openReassign}>
                改派班组
              </button>
            ) : null}
            <button type="button" className="btn btn-danger" onClick={() => setConfirmAction('delete')}>
              删除
            </button>
          </>
        }
      />

      <StateBlock loading={detail.loading} error={detail.error} onRetry={detail.reload} empty={!task} emptyText="任务不存在">
        {task ? (
          <>
            <div className="alert alert-info">
              <p>
                状态流转：待开工 → 清淤中（首次录入清淤记录自动推进）→ 待验收（完工报验，需至少 1 条清淤记录）→
                已验收（验收合格）；验收结论为需整改时任务回到清淤中，登记整改完成后可重新报验。
              </p>
            </div>

            <SectionCard title="任务信息" subtitle={`创建于 ${formatDateTime(task.createdAt)}，最近更新 ${formatDateTime(task.updatedAt)}`}>
              <InfoList
                items={[
                  { label: '任务编号', value: task.code },
                  { label: '任务标题', value: task.title },
                  { label: '关联管段', value: detail.data?.segment ? `${detail.data.segment.code} · ${detail.data.segment.name}` : '—' },
                  { label: '所属片区', value: detail.data?.segment?.district ?? '—' },
                  { label: '优先级', value: <StatusTag list="taskPriorities" value={task.priority} /> },
                  { label: '任务来源', value: <StatusTag list="taskSources" value={task.source} /> },
                  { label: '计划清淤方式', value: <StatusTag list="cleaningMethods" value={task.method} /> },
                  { label: '计划开始日期', value: formatDate(task.planStartDate) },
                  { label: '计划完成日期', value: formatDate(task.planEndDate) },
                  { label: '实施班组', value: task.teamName || '—' },
                  { label: '现场负责人', value: task.leaderName || '—' },
                  { label: '联系电话', value: task.leaderPhone || '—' },
                  { label: '实际开工时间', value: formatDateTime(task.startedAt) },
                  { label: '完工时间', value: formatDateTime(task.finishedAt) },
                  { label: '验收时间', value: formatDateTime(task.acceptedAt) },
                  { label: '任务说明', value: task.description || '—', span: 3 },
                  { label: '取消原因', value: task.cancelReason || '—', span: 3 }
                ]}
              />
            </SectionCard>

            <SectionCard
              title="清淤记录汇总"
              subtitle="任务下所有清淤记录的汇总口径"
              extra={
                can('start') || can('complete') || can('edit') ? (
                  <Link className="btn btn-primary btn-sm" to={`/records/new?taskId=${id}`}>
                    录入清淤记录
                  </Link>
                ) : (
                  <Link className="link" to={`/records?taskId=${id}`}>
                    查看记录列表
                  </Link>
                )
              }
            >
              <div className="stat-grid">
                <StatCard label="记录条数" value={formatNumber(totals?.recordCount ?? 0, 0)} tone="primary" />
                <StatCard label="累计清淤量" value={formatVolume(totals?.sludgeVolumeM3 ?? 0)} />
                <StatCard label="累计清淤长度" value={formatLength(totals?.cleanedLengthM ?? 0)} />
                <StatCard label="最近清淤日期" value={formatDate(totals?.latestCleanedAt)} />
              </div>
            </SectionCard>

            <SectionCard title="清淤记录明细" subtitle="该任务下最近 50 条清淤记录">
              <div className="card-body-flush">
                <DataTable
                  columns={recordColumns}
                  rows={records.data?.list ?? []}
                  rowKey={(row) => row.id}
                  loading={records.loading}
                  error={records.error}
                  onRetry={records.reload}
                  emptyText="该任务还没有清淤记录"
                  emptyDescription="录入第一条清淤记录后，任务会自动从待开工推进为清淤中。"
                />
              </div>
            </SectionCard>

            <SectionCard
              title="班组工作量归属"
              subtitle="按每条清淤记录的实际作业日期归属到当时班组，与班组工作量统计、看板同一口径；跨日改派不会重复计算"
            >
              <div className="card-body-flush">
                <DataTable
                  columns={teamColumns}
                  rows={teamWorkload}
                  rowKey={(row) => row.teamName}
                  emptyText="暂无可归属的工作量"
                  emptyDescription="任务录入清淤记录后，按作业日期归属到当时实施的班组。"
                />
              </div>
            </SectionCard>

            <SectionCard
              title="改派记录"
              subtitle="记录每次班组改派的原班组、接手班组、原因与生效日期"
              extra={
                can('reassign') ? (
                  <button type="button" className="btn btn-primary btn-sm" onClick={openReassign}>
                    改派班组
                  </button>
                ) : null
              }
            >
              <div className="card-body-flush">
                <DataTable
                  columns={reassignmentColumns}
                  rows={reassignments}
                  rowKey={(row) => row.id}
                  emptyText="该任务尚未改派过班组"
                />
              </div>
            </SectionCard>

            <SectionCard
              title="验收结论"
              subtitle="任务完工报验后的验收结果（需整改时任务回到清淤中）"
              extra={
                acceptance ? (
                  <Link className="link" to={`/acceptances/${acceptance.id}`}>
                    查看验收记录
                  </Link>
                ) : null
              }
            >
              {acceptance ? (
                <InfoList
                  items={[
                    { label: '验收编号', value: acceptance.code },
                    { label: '验收结论', value: <StatusTag list="acceptanceResults" value={acceptance.result} /> },
                    { label: '验收日期', value: formatDate(acceptance.acceptedAt) },
                    { label: '验收评分', value: `${acceptance.score} 分` },
                    { label: '验收人', value: acceptance.inspectorName },
                    { label: '验收单位', value: acceptance.inspectorOrg || '—' },
                    { label: '整改期限', value: formatDate(acceptance.rectifyDeadline) },
                    { label: '整改完成日期', value: formatDate(acceptance.rectifiedAt) },
                    { label: '存在问题', value: acceptance.issues || '—', span: 3 }
                  ]}
                />
              ) : (
                <p className="form-note">该任务尚无验收记录，完工报验后可在「验收记录」模块登记验收结论。</p>
              )}
            </SectionCard>
          </>
        ) : null}
      </StateBlock>

      <ConfirmDialog
        open={confirmAction === 'complete'}
        title="完工报验"
        busy={busy}
        confirmText="确认报验"
        message={<p>报验前请确认清淤记录已录入完整；报验后任务进入「待验收」，需由验收人登记验收结论。</p>}
        onConfirm={() => void runAction('任务已完工报验', () => taskApi.complete(id))}
        onCancel={() => setConfirmAction(null)}
      />

      <ConfirmDialog
        open={confirmAction === 'delete'}
        title="删除清淤任务"
        danger
        busy={busy}
        confirmText="确认删除"
        message={<p>已录入清淤记录或已产生验收记录的任务不允许删除。</p>}
        onConfirm={handleDelete}
        onCancel={() => setConfirmAction(null)}
      />

      <Modal
        open={cancelOpen}
        title="取消清淤任务"
        onClose={() => setCancelOpen(false)}
        footer={
          <>
            <button type="button" className="btn btn-ghost" onClick={() => setCancelOpen(false)}>
              放弃
            </button>
            <button type="button" className="btn btn-danger" disabled={busy} onClick={() => void submitCancel()}>
              {busy ? '处理中…' : '确认取消任务'}
            </button>
          </>
        }
      >
        <div className="form-field">
          <span className="form-label">
            取消原因
            <em className="form-required">*</em>
          </span>
          <textarea
            className="textarea"
            value={cancelReason}
            placeholder="例如：现场条件不具备，顺延至汛期后实施"
            onChange={(event) => setCancelReason(event.target.value)}
          />
          {cancelError ? <span className="form-error">{cancelError}</span> : null}
        </div>
        <p className="form-note">任务取消后不可再恢复，也不能继续录入清淤记录。</p>
      </Modal>

      <Modal
        open={reassignOpen}
        title="改派实施班组"
        width={560}
        onClose={() => setReassignOpen(false)}
        footer={
          <>
            <button type="button" className="btn btn-ghost" onClick={() => setReassignOpen(false)}>
              放弃
            </button>
            <button type="button" className="btn btn-primary" disabled={busy} onClick={() => void submitReassign()}>
              {busy ? '提交中…' : '确认改派'}
            </button>
          </>
        }
      >
        <div className="alert alert-info">
          <p>
            当前班组为 <strong>{task?.teamName || '—'}</strong>。改派后任务交接给新班组，
            工作量按清淤记录的<strong>实际作业日期</strong>归属：生效日期之前的工作量仍计入原班组，
            之后的计入接手班组，不会两边各算一遍。已完工报验的任务不能改派。
          </p>
        </div>
        <div className="form-grid">
          <div className="form-field">
            <span className="form-label">
              接手班组
              <em className="form-required">*</em>
            </span>
            <input
              className="input"
              value={toTeam}
              placeholder="例如 城西养护二班"
              onChange={(event) => setToTeam(event.target.value)}
            />
          </div>
          <div className="form-field">
            <span className="form-label">
              生效日期
              <em className="form-required">*</em>
            </span>
            <input
              className="input"
              type="date"
              max={today()}
              value={effectiveDate}
              onChange={(event) => setEffectiveDate(event.target.value)}
            />
          </div>
          <div className="form-field">
            <span className="form-label">操作人</span>
            <input
              className="input"
              value={operator}
              placeholder="可选"
              onChange={(event) => setOperator(event.target.value)}
            />
          </div>
          <div className="form-field" style={{ gridColumn: 'span 2' }}>
            <span className="form-label">
              改派原因
              <em className="form-required">*</em>
            </span>
            <textarea
              className="textarea"
              value={reassignReason}
              placeholder="例如 原班组设备检修，剩余作业移交接手班组"
              onChange={(event) => setReassignReason(event.target.value)}
            />
          </div>
        </div>
        {reassignError ? (
          <p className="form-error" style={{ marginTop: 8 }}>
            {reassignError}
          </p>
        ) : null}
        <p className="form-note">若目标月份的班组工作量月报已出具并封账，改派会被拒绝以保证月报不被改写。</p>
      </Modal>
    </div>
  );
}
