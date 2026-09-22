// 班组工作量统计：按清淤记录实际作业日期归属到当时班组，与任务详情、看板同一口径。
// 支持按月查看并出具（封账）月报；月报封账后改派不能再改写该月归属。
import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { teamApi } from '../../api/team';
import { DataTable, type Column } from '../../components/DataTable';
import { Modal } from '../../components/Modal';
import { PageHeader } from '../../components/PageHeader';
import { SectionCard } from '../../components/SectionCard';
import { StateBlock } from '../../components/StateBlock';
import { useToast } from '../../components/Toast';
import { useAsync } from '../../hooks/useAsync';
import type { TeamWorkload } from '../../types/domain';
import { formatLength, formatNumber, formatVolume } from '../../utils/format';

function currentMonth(): string {
  return new Date().toISOString().slice(0, 7);
}

export function TeamWorkloadPage() {
  const navigate = useNavigate();
  const toast = useToast();
  const [month, setMonth] = useState(currentMonth());

  const workload = useAsync(() => teamApi.workload(month), [month]);
  const reports = useAsync(() => teamApi.reports(), []);

  const [publishOpen, setPublishOpen] = useState(false);
  const [busy, setPublishBusy] = useState(false);
  const [publishedBy, setPublishedBy] = useState('');
  const [remark, setRemark] = useState('');

  const data = workload.data;
  const items = data?.items ?? [];

  const totals = useMemo(() => {
    return items.reduce(
      (acc, item) => {
        acc.records += item.recordCount;
        acc.personDays += item.personDays;
        acc.sludge += item.sludgeVolumeM3;
        acc.length += item.cleanedLengthM;
        return acc;
      },
      { records: 0, personDays: 0, sludge: 0, length: 0 }
    );
  }, [items]);

  const submitPublish = async () => {
    setPublishBusy(true);
    try {
      await teamApi.publish({ month, remark: remark.trim() || undefined, publishedBy: publishedBy.trim() || undefined });
      toast.success(`${month} 月班组工作量月报已出具并封账`);
      setPublishOpen(false);
      workload.reload();
      reports.reload();
    } catch (cause: unknown) {
      toast.error(cause instanceof Error ? cause.message : '出具月报失败');
    } finally {
      setPublishBusy(false);
    }
  };

  const columns: Column<TeamWorkload>[] = [
    { key: 'teamName', title: '班组', render: (row) => <span className="cell-main">{row.teamName}</span> },
    { key: 'recordCount', title: '清淤记录条数', align: 'right', render: (row) => formatNumber(row.recordCount, 0) },
    { key: 'personDays', title: '作业工日', align: 'right', render: (row) => formatNumber(row.personDays, 0) },
    { key: 'sludgeVolumeM3', title: '清淤量', align: 'right', render: (row) => formatVolume(row.sludgeVolumeM3) },
    { key: 'cleanedLengthM', title: '清淤长度', align: 'right', render: (row) => formatLength(row.cleanedLengthM) }
  ];

  const published = data?.published ?? false;

  return (
    <div className="page">
      <PageHeader
        title="班组工作量统计"
        description="按清淤记录的实际作业日期归属到当时实施的班组，与任务详情、运行看板同一口径，跨日改派不重复计算。"
        actions={
          <>
            <button type="button" className="btn btn-ghost" onClick={() => navigate('/')}>
              返回看板
            </button>
            <button
              type="button"
              className={published ? 'btn btn-ghost' : 'btn btn-primary'}
              onClick={() => {
                setPublishedBy('');
                setRemark('');
                setPublishOpen(true);
              }}
            >
              {published ? '查看/补注月报' : '出具月报（封账）'}
            </button>
          </>
        }
      />

      <SectionCard
        title={`${month} 月班组工作量`}
        subtitle={published ? '该月月报已出具并封账，改派不会再改写本月归属' : '本月月报尚未封账'}
        extra={
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            {published ? <span className="tag tag-success">已封账</span> : <span className="tag tag-muted">未封账</span>}
            <input
              className="input"
              type="month"
              value={month}
              max={currentMonth()}
              onChange={(event) => setMonth(event.target.value || currentMonth())}
            />
          </div>
        }
      >
        <StateBlock loading={workload.loading} error={workload.error} onRetry={workload.reload}>
          <div className="stat-grid">
            <div className="stat-card stat-primary">
              <p className="stat-label">参与班组</p>
              <p className="stat-value">{formatNumber(items.length, 0)}</p>
            </div>
            <div className="stat-card">
              <p className="stat-label">清淤记录</p>
              <p className="stat-value">{formatNumber(totals.records, 0)}</p>
            </div>
            <div className="stat-card">
              <p className="stat-label">作业工日</p>
              <p className="stat-value">{formatNumber(totals.personDays, 0)}</p>
            </div>
            <div className="stat-card">
              <p className="stat-label">清淤量合计</p>
              <p className="stat-value">{formatVolume(totals.sludge)}</p>
            </div>
          </div>
          <div style={{ height: 16 }} />
          <div className="card-body-flush">
            <DataTable
              columns={columns}
              rows={items}
              rowKey={(row) => row.teamName}
              emptyText="本月暂无班组工作量"
              emptyDescription="该月份没有清淤记录，或记录尚未归属到具体班组。"
            />
          </div>
        </StateBlock>
      </SectionCard>

      <SectionCard title="已出具月报" subtitle="封账后的月份不受后续改派影响">
        <div className="card-body-flush">
          <DataTable
            columns={[
              { key: 'month', title: '月份', render: (row) => <span className="cell-main">{row.month}</span> },
              {
                key: 'published',
                title: '状态',
                render: (row) =>
                  row.published ? <span className="tag tag-success">已封账</span> : <span className="tag tag-muted">草稿</span>
              },
              { key: 'publishedBy', title: '出具人', render: (row) => row.publishedBy || '—' },
              { key: 'remark', title: '备注', render: (row) => row.remark || '—' },
              {
                key: 'action',
                title: '操作',
                render: (row) => (
                  <button type="button" className="btn-link" onClick={() => setMonth(row.month)}>
                    查看该月
                  </button>
                )
              }
            ]}
            rows={reports.data ?? []}
            rowKey={(row) => row.id}
            loading={reports.loading}
            error={reports.error}
            onRetry={reports.reload}
            emptyText="尚未出具过月报"
          />
        </div>
      </SectionCard>

      <Modal
        open={publishOpen}
        title={`出具 ${month} 月班组工作量月报`}
        onClose={() => setPublishOpen(false)}
        footer={
          <>
            <button type="button" className="btn btn-ghost" onClick={() => setPublishOpen(false)}>
              取消
            </button>
            <button type="button" className="btn btn-primary" disabled={busy} onClick={() => void submitPublish()}>
              {busy ? '提交中…' : '确认出具并封账'}
            </button>
          </>
        }
      >
        <div className="alert alert-info">
          <p>封账后，该月各班组的工作量归属即被冻结：之后任何会改变本月归属的改派都会被系统拒绝，月报不会被改写。</p>
        </div>
        <div className="form-field">
          <span className="form-label">出具人</span>
          <input className="input" value={publishedBy} placeholder="可选" onChange={(e) => setPublishedBy(e.target.value)} />
        </div>
        <div className="form-field">
          <span className="form-label">备注</span>
          <textarea className="textarea" value={remark} onChange={(e) => setRemark(e.target.value)} />
        </div>
      </Modal>
    </div>
  );
}
