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
import type { RecordListItem, TaskAction, TeamAssignment, TeamWorkload } from '../../types/domain';
import { formatDate, formatDateTime, formatLength, formatNumber, formatVolume } from '../../utils/format';

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
  const [targetTeamName, setTargetTeamName] = useState('');
  const [reassignReason, setReassignReason] = useState('');
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

  const submitReassign = async () => {
    if (!task) {
      return;
    }
    if (!targetTeamName.trim()) {
      setReassignError('请填写接手班组');
      return;
    }
    if (!reassignReason.trim()) {
      setReassignError('请填写改派原因');
      return;
    }
    setBusy(true);
    try {
      await taskApi.reassign(id, {
        currentTeamName: task.teamName || '未指定班组',
        targetTeamName: targetTeamName.trim(),
        reason: reassignReason.trim()
      });
      toast.success('班组已改派，工作量将按实际作业日期归属');
      setReassignOpen(false);
      setTargetTeamName('');
      setReassignReason('');
      setReassignError('');
      detail.reload();
      records.reload();
    } catch (cause: unknown) {
      toast.error(toErrorMessage(cause));
    } finally {
      setBusy(false);
    }
  };

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

  const workloadColumns: Column<TeamWorkload>[] = [
    { key: 'teamName', title: '归属班组', render: (row) => row.teamName || '未指定班组' },
    { key: 'recordCount', title: '记录数', width: '90px', align: 'right', render: (row) => formatNumber(row.recordCount, 0) },
    { key: 'personnelCount', title: '作业人次', width: '100px', align: 'right', render: (row) => formatNumber(row.personnelCount, 0) },
    { key: 'actualWorkHours', title: '作业工时', width: '100px', align: 'right', render: (row) => formatNumber(row.actualWorkHours, 1) },
    { key: 'lengthM', title: '清淤长度', width: '110px', align: 'right', render: (row) => formatLength(row.lengthM) },
    { key: 'sludgeVolumeM3', title: '清淤量', width: '110px', align: 'right', render: (row) => formatVolume(row.sludgeVolumeM3) },
    { key: 'waterVolumeM3', title: '用水量', width: '110px', align: 'right', render: (row) => formatVolume(row.waterVolumeM3) }
  ];

  const assignmentColumns: Column<TeamAssignment>[] = [
    { key: 'teamName', title: '班组', width: '140px', render: (row) => row.teamName },
    {
      key: 'changeType',
      title: '类型',
      width: '90px',
      render: (row) => (
        <span className={`tag ${row.changeType === 'reassign' ? 'tag-warn' : 'tag-muted'}`}>
          {row.changeType === 'reassign' ? '改派接手' : '原班组派工'}
        </span>
      )
    },
    { key: 'effectiveDate', title: '生效日期', width: '110px', render: (row) => formatDate(row.effectiveDate) },
    { key: 'reason', title: '原因', render: (row) => row.reason || '—' },
    { key: 'operatorName', title: '操作人', width: '100px', render: (row) => row.operatorName || '—' },
    { key: 'createdAt', title: '操作时间', width: '160px', render: (row) => formatDateTime(row.createdAt) }
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
              <button type="button" className="btn btn-ghost" onClick={() => setReassignOpen(true)}>
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

            <SectionCard title="班组工作量归属" subtitle="按每条清淤记录的实际作业日期归属；跨日期改派不会重复计算同一批工作量">
              <div className="card-body-flush">
                <DataTable
                  columns={workloadColumns}
                  rows={detail.data?.teamWorkloads ?? []}
                  rowKey={(row) => row.teamName}
                  emptyText="暂无已录入工作量"
                />
              </div>
            </SectionCard>

            <SectionCard title="班组派工 / 改派记录" subtitle="记录原班组、接手班组、改派原因与操作时间">
              <div className="card-body-flush">
                <DataTable
                  columns={assignmentColumns}
                  rows={detail.data?.assignments ?? []}
                  rowKey={(row) => row.id}
                  emptyText="暂无派工记录"
                />
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
        title="改派班组"
        onClose={() => setReassignOpen(false)}
        footer={
          <>
            <button type="button" className="btn btn-ghost" onClick={() => setReassignOpen(false)}>
              放弃
            </button>
            <button type="button" className="btn btn-primary" disabled={busy} onClick={() => void submitReassign()}>
              {busy ? '处理中…' : '确认改派'}
            </button>
          </>
        }
      >
        <div className="form-field">
          <span className="form-label">原班组</span>
          <input className="input" value={task?.teamName || '未指定班组'} disabled />
        </div>
        <div className="form-field">
          <span className="form-label">
            接手班组
            <em className="form-required">*</em>
          </span>
          <input
            className="input"
            value={targetTeamName}
            placeholder="例如：城西养护二班"
            onChange={(event) => setTargetTeamName(event.target.value)}
          />
        </div>
        <div className="form-field">
          <span className="form-label">
            改派原因
            <em className="form-required">*</em>
          </span>
          <textarea
            className="textarea"
            value={reassignReason}
            placeholder="例如：原班组设备调度冲突，由具备吸污车的班组接手"
            onChange={(event) => setReassignReason(event.target.value)}
          />
          {reassignError ? <span className="form-error">{reassignError}</span> : null}
        </div>
        <p className="form-note">
          改派当天及以后的工作量归属接手班组，改派前已完成的清淤记录仍归属原班组；完工报验后不可改派。
        </p>
      </Modal>
    </div>
  );
}
