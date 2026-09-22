import type { TeamWorkloadResponse, WorkloadReport } from '../types/domain';
import { buildQuery, http } from './client';

export interface PublishMonthPayload {
  month: string;
  remark?: string;
  publishedBy?: string;
}

export const teamApi = {
  workload: (month?: string) =>
    http.get<TeamWorkloadResponse>(`/team-workload${buildQuery({ month })}`),
  reports: () => http.get<WorkloadReport[]>('/team-workload/reports'),
  publish: (payload: PublishMonthPayload) =>
    http.post<WorkloadReport>('/team-workload/reports', payload)
};
