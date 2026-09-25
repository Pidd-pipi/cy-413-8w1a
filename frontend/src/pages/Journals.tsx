import {Button, Card, Checkbox, Col, Form, Input, Row, Select, Slider, Tag, Tooltip, Typography, message} from 'antd';
import {useEffect, useState} from 'react';
import {deleteJournal, listJournals, saveJournal, updateJournal} from '../api/journal';
import {EmptyState} from '../components/common/EmptyState';
import {MoodCard} from '../components/common/MoodCard';
import {MoodSelector} from '../components/common/MoodSelector';
import type {Journal, JournalPayload, MoodTag} from '../types';

export function Journals() {
	const [items, setItems] = useState<Journal[]>([]);
	const [tags, setTags] = useState<MoodTag[]>(['calm']);
	const [editingId, setEditingId] = useState<number | null>(null);
	const [form] = Form.useForm();
	const load = () => listJournals().then(setItems).catch(e => message.error(e.message));
	useEffect(() => {
		load();
	}, []);

	const startCreate = () => {
		setEditingId(null);
		setTags(['calm']);
		form.setFieldsValue({title: '', content: '', mood_level: 7, weather: '晴', is_private: true});
	};
	const startEdit = (j: Journal) => {
		let nextTags: MoodTag[] = j.mood_tags?.length ? j.mood_tags : (j.linked_mood ? parseTags(j.linked_mood.mood_tags) : ['calm']);
		setEditingId(j.id);
		setTags(nextTags);
		form.setFieldsValue({
			title: j.title,
			content: j.content,
			mood_level: j.mood_level || 7,
			weather: j.weather || '晴',
			is_private: j.is_private,
		});
		window.scrollTo({top: 0, behavior: 'smooth'});
	};

	const onFinish = async (v: Record<string, unknown>) => {
		const payload: JournalPayload = {
			title: v.title as string,
			content: v.content as string,
			mood_level: v.mood_level as number,
			mood_tags: tags,
			weather: v.weather as string,
			is_private: Boolean(v.is_private),
		};
		try {
			if (editingId) {
				await updateJournal(editingId, payload);
				message.success('日记已更新，关联的情绪记录已同步');
			} else {
				await saveJournal(payload);
				message.success('日记已保存，当天的情绪记录已一并记好');
			}
			startCreate();
			load();
		} catch (e) {
			message.error((e as Error).message);
		}
	};

	return <>
		<Typography.Title>日记本</Typography.Title>
		<Row gutter={[20, 20]}>
			<Col xs={24} lg={10}>
				<Card title={editingId ? '编辑这一页日记' : '写一页日记'} extra={editingId ? <Button size="small" onClick={startCreate}>写新的</Button> : null}>
					<Form form={form} layout="vertical" initialValues={{mood_level: 7, is_private: true, weather: '晴'}} onFinish={onFinish}>
						<Form.Item name="title" label="标题" rules={[{required: true, message: '给今天一句标题'}]}>
							<Input placeholder="给今天一句标题"/>
						</Form.Item>
						<Form.Item name="content" label="正文（支持 Markdown 风格文本）" rules={[{required: true, message: '写点今天发生了什么'}]}>
							<Input.TextArea rows={8} placeholder="今天发生了什么？"/>
						</Form.Item>
						<Form.Item name="mood_level" label="心情指数">
							<Slider min={1} max={10}/>
						</Form.Item>
						<Form.Item label="情绪标签（可多选，保存时会同步到当天情绪记录）">
							<MoodSelector value={tags} onChange={setTags}/>
						</Form.Item>
						<Form.Item name="weather" label="天气">
							<Select options={['晴', '阴', '雨', '风'].map(v => ({value: v}))}/>
						</Form.Item>
						<Form.Item name="is_private" valuePropName="checked">
							<Checkbox>仅自己可见</Checkbox>
						</Form.Item>
						<Button type="primary" htmlType="submit">{editingId ? '保存修改' : '保存日记'}</Button>
					</Form>
				</Card>
			</Col>
			<Col xs={24} lg={14}>
				<Card title="时间轴">
					{items.length ? <div className="timeline">
						{items.map(j => <JournalTimelineItem key={j.id} j={j} onEdit={() => startEdit(j)} onDeleted={load}/>)}
					</div> : <EmptyState title="写下第一篇日记，和自己好好聊聊"/>}
				</Card>
			</Col>
		</Row>
	</>;
}

function parseTags(raw: string): MoodTag[] {
	try {
		const parsed = JSON.parse(raw);
		return Array.isArray(parsed) ? parsed : [];
	} catch {
		return [];
	}
}

function JournalTimelineItem({j, onEdit, onDeleted}: {j: Journal; onEdit: () => void; onDeleted: () => void}) {
	return <article className="journal">
		<div>
			<Tag color="purple">{j.created_at.slice(0, 10)}</Tag>
			<b>{j.title}</b>
			<span className="muted"> · {j.weather} · 心情 {j.mood_level}/10</span>
		</div>
		<p>{j.content}</p>
		<div className="journal-mood">
			{j.linked_mood
				? <><div className="journal-mood__flag"><Tag color="green">已关联情绪记录</Tag></div><MoodCard mood={j.linked_mood}/></>
				: j.day_has_mood
					? <Tooltip title="当天已有情绪记录（手动记录或其他日记生成），这篇日记不会覆盖它">
						<Tag color="gold">当天已有情绪记录 · 未关联</Tag>
					</Tooltip>
					: <Tooltip title="这篇日记没有生成或同步情绪记录">
						<Tag>暂无关联情绪记录</Tag>
					</Tooltip>}
		</div>
		<div style={{marginTop: 8}}>
			<Button size="small" onClick={onEdit} style={{marginRight: 8}}>编辑</Button>
			<Button danger size="small" onClick={async () => {
				try {
					await deleteJournal(j.id);
					message.success('日记已删除，情绪记录仍保留');
					onDeleted();
				} catch (e) {
					message.error((e as Error).message);
				}
			}}>删除</Button>
		</div>
	</article>;
}
